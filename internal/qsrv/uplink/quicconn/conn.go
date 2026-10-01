package quicconn

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	quic "github.com/quic-go/quic-go"

	"github.com/jaywehosl/qd/internal/qsrv/uplink/relay"
	"github.com/jaywehosl/qd/internal/roots"
)

const defaultMaxDatagram = 1200

const udpBufSize = 4 << 20

func setUDPBuffers(pc *net.UDPConn) {
	_ = pc.SetReadBuffer(udpBufSize)
	_ = pc.SetWriteBuffer(udpBufSize)
}

type transportSocket struct {
	tr *quic.Transport
	pc net.PacketConn
}

type Conn struct {
	qc *quic.Conn

	mu       sync.Mutex
	tr       *quic.Transport
	pc       net.PacketConn
	prev     []transportSocket
	remote   net.Addr
	keep     func(fd uintptr)
	maxDgram atomic.Int64
}

func (c *Conn) SendDatagram(b []byte) error {
	err := c.qc.SendDatagram(b)
	var tooLarge *quic.DatagramTooLargeError
	if errors.As(err, &tooLarge) {
		c.maxDgram.Store(tooLarge.MaxDatagramPayloadSize)
	}
	return err
}

func (c *Conn) MaxDatagramSize() int {
	if v := c.maxDgram.Load(); v > 0 {
		return int(v)
	}
	return defaultMaxDatagram
}

func (c *Conn) QUIC() *quic.Conn { return c.qc }

func (c *Conn) Migrate(ctx context.Context, laddr *net.UDPAddr) error {
	pc, err := c.listenLike(laddr)
	if err != nil {
		return err
	}
	setUDPBuffers(pc)
	keepOutside(pc, c.keep)
	newTr := &quic.Transport{Conn: pc}

	path, err := c.qc.AddPath(newTr)
	if err != nil {
		newTr.Close()
		pc.Close()
		return err
	}
	if err := path.Probe(ctx); err != nil {
		path.Close()
		c.park(newTr, pc)
		return err
	}
	if err := path.Switch(); err != nil {
		path.Close()
		c.park(newTr, pc)
		return err
	}

	c.mu.Lock()
	c.prev = append(c.prev, transportSocket{tr: c.tr, pc: c.pc})
	c.tr, c.pc = newTr, pc
	c.mu.Unlock()

	return nil
}

func (c *Conn) Close() error {
	err := c.qc.CloseWithError(0, "")
	c.mu.Lock()
	tr := c.tr
	pc := c.pc
	prev := c.prev
	c.prev = nil
	c.mu.Unlock()
	if tr != nil {
		tr.Close()
	}
	if pc != nil {
		pc.Close()
	}
	for _, ts := range prev {
		if ts.tr != nil {
			ts.tr.Close()
		}
		if ts.pc != nil {
			ts.pc.Close()
		}
	}
	return err
}

type Dialer struct {
	TLS  *tls.Config
	QUIC *quic.Config
	Keep func(fd uintptr)
}

func (d Dialer) Dial(ctx context.Context, endpoint string) (*Conn, error) {
	addrs, err := resolve(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	if len(addrs) == 1 {
		conn, err := d.reach(ctx, addrs[0])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", addrs[0], err)
		}
		return conn, nil
	}

	round, stop := context.WithCancel(ctx)
	defer stop()

	type finish struct {
		conn *Conn
		err  error
	}
	line := make(chan finish, len(addrs))

	for i, raddr := range addrs {
		go func(n int, where *net.UDPAddr) {
			if n > 0 {
				select {
				case <-time.After(time.Duration(n) * headStart):
				case <-round.Done():
					line <- finish{err: round.Err()}
					return
				}
			}
			conn, err := d.reach(round, where)
			if err != nil {
				line <- finish{err: fmt.Errorf("%s: %w", where, err)}
				return
			}
			line <- finish{conn: conn}
		}(i, raddr)
	}

	tried := make([]string, 0, len(addrs))
	for i := range addrs {
		got := <-line
		if got.err == nil {
			go func(left int) {
				for ; left > 0; left-- {
					if late := <-line; late.conn != nil {
						late.conn.Close()
					}
				}
			}(len(addrs) - i - 1)
			return got.conn, nil
		}
		tried = append(tried, got.err.Error())
	}
	return nil, errors.New(strings.Join(tried, " / "))
}

func (d Dialer) reach(ctx context.Context, raddr *net.UDPAddr) (*Conn, error) {
	pc, err := listenFor(raddr)
	if err != nil {
		fmt.Printf("dial     %s: no socket: %v\n", raddr, err)
		return nil, err
	}
	setUDPBuffers(pc)
	keepOutside(pc, d.Keep)
	fmt.Printf("dial     %s from %s\n", raddr, pc.LocalAddr())
	tr := &quic.Transport{Conn: pc}

	began := time.Now()
	qc, err := dialOn(ctx, tr, raddr, d.TLS, d.QUIC)
	if err != nil {
		fmt.Printf("dial     %s gave up after %d ms: %v\n", raddr, time.Since(began).Milliseconds(), err)
		tr.Close()
		pc.Close()
		return nil, err
	}
	fmt.Printf("dial     %s answered in %d ms\n", raddr, time.Since(began).Milliseconds())
	c := &Conn{qc: qc, tr: tr, pc: pc, remote: raddr, keep: d.Keep}
	c.maxDgram.Store(defaultMaxDatagram)
	return c, nil
}

const headStart = 250 * time.Millisecond

func OverRelay(ctx context.Context, sess *relay.Session, authority string, conf *quic.Config, tickets tls.ClientSessionCache) (*Conn, error) {
	host, _, err := net.SplitHostPort(authority)
	if err != nil {
		return nil, err
	}
	pc := relay.NewPacketConn(sess)
	if err := sess.Start(); err != nil {
		return nil, err
	}
	if err := sess.Ready(ctx); err != nil {
		sess.Stop()
		return nil, err
	}
	qc, err := DialPacketConn(ctx, pc, relay.Peer, &tls.Config{ServerName: host, ClientSessionCache: tickets, RootCAs: roots.Pool()}, conf)
	if err != nil {
		sess.Stop()
		return nil, err
	}
	return qc, nil
}

func DialPacketConn(ctx context.Context, pc net.PacketConn, raddr net.Addr, tlsConf *tls.Config, quicConf *quic.Config) (*Conn, error) {
	tr := &quic.Transport{Conn: pc}
	qc, err := dialOn(ctx, tr, raddr, tlsConf, quicConf)
	if err != nil {
		tr.Close()
		return nil, err
	}
	c := &Conn{qc: qc, tr: tr, pc: pc, remote: raddr}
	c.maxDgram.Store(defaultMaxDatagram)
	return c, nil
}

var Tokens quic.TokenStore

func DefaultConfig() *quic.Config {
	return &quic.Config{
		TokenStore:                     Tokens,
		EnableDatagrams:                true,
		MaxIdleTimeout:                 90 * time.Second,
		KeepAlivePeriod:                15 * time.Second,
		InitialStreamReceiveWindow:     2 << 20,
		MaxStreamReceiveWindow:         6 << 20,
		InitialConnectionReceiveWindow: 3 << 20,
		MaxConnectionReceiveWindow:     15 << 20,
	}
}

func configOrDefault(c *quic.Config) *quic.Config {
	if c == nil {
		return DefaultConfig()
	}
	return c
}

func ensureALPN(t *tls.Config) *tls.Config {
	if t == nil {
		t = &tls.Config{}
	} else {
		t = t.Clone()
	}
	if len(t.NextProtos) == 0 {
		t.NextProtos = []string{"h3"}
	}
	return t
}

func (c *Conn) park(tr *quic.Transport, pc *net.UDPConn) {
	c.mu.Lock()
	c.prev = append(c.prev, transportSocket{tr: tr, pc: pc})

	stale := []transportSocket{}
	if over := len(c.prev) - pathKeep; over > 0 {
		stale = append(stale, c.prev[:over]...)
		c.prev = append([]transportSocket{}, c.prev[over:]...)
	}
	c.mu.Unlock()

	for _, one := range stale {
		if one.tr != nil {
			one.tr.Close()
		}
		if one.pc != nil {
			one.pc.Close()
		}
	}
}

const pathKeep = 6

func keepOutside(pc *net.UDPConn, keep func(fd uintptr)) {
	if keep == nil {
		return
	}
	raw, err := pc.SyscallConn()
	if err != nil {
		return
	}
	raw.Control(keep)
}

var known sync.Map

func Addrs(ctx context.Context, endpoint string) ([]string, error) {
	held, err := resolve(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(held))
	for _, one := range held {
		out = append(out, one.String())
	}
	return out, nil
}

func resolve(ctx context.Context, endpoint string) ([]*net.UDPAddr, error) {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, err
	}
	number, err := net.LookupPort("udp", port)
	if err != nil {
		return nil, err
	}

	if ip, err := netip.ParseAddr(host); err == nil {
		return []*net.UDPAddr{{IP: ip.AsSlice(), Port: number}}, nil
	}

	if held, ok := known.Load(endpoint); ok {
		go refresh(endpoint, host, number)
		return held.([]*net.UDPAddr), nil
	}

	look, stop := context.WithTimeout(ctx, resolveWait)
	ips, err := net.DefaultResolver.LookupIP(look, "ip", host)
	stop()
	if err != nil || len(ips) == 0 {
		if held, ok := known.Load(endpoint); ok {
			return held.([]*net.UDPAddr), nil
		}
		if err == nil {
			err = fmt.Errorf("no address for %s", host)
		}
		return nil, err
	}

	out := order(ips, number)
	known.Store(endpoint, out)
	fmt.Printf("resolve  %s -> %v (v6 route %v)\n", endpoint, out, holdsV6())
	return out, nil
}

func order(ips []net.IP, port int) []*net.UDPAddr {
	v6 := holdsV6()

	first := make([]*net.UDPAddr, 0, len(ips))
	rest := make([]*net.UDPAddr, 0, len(ips))
	for _, ip := range ips {
		one := &net.UDPAddr{IP: ip, Port: port}
		if ip.To4() != nil || v6 {
			first = append(first, one)
			continue
		}
		rest = append(rest, one)
	}
	return append(first, rest...)
}

func holdsV6() bool {
	c, err := net.Dial("udp6", "[2001:4860:4860::8888]:53")
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func listenFor(raddr *net.UDPAddr) (*net.UDPConn, error) {
	network := "udp6"
	if raddr.IP.To4() != nil {
		network = "udp4"
	}
	return net.ListenUDP(network, &net.UDPAddr{Port: 0})
}

func (c *Conn) listenLike(laddr *net.UDPAddr) (*net.UDPConn, error) {
	network := "udp"
	if remote, ok := c.remote.(*net.UDPAddr); ok && remote != nil {
		if remote.IP.To4() != nil {
			network = "udp4"
		} else {
			network = "udp6"
		}
	}
	if laddr == nil {
		laddr = &net.UDPAddr{Port: 0}
	}
	return net.ListenUDP(network, laddr)
}

const resolveWait = 4 * time.Second

var (
	skipV2 atomic.Bool
	prove  atomic.Bool
	ECH    func(serverName string) []byte
	noECH  sync.Map
)

type echFailure struct {
	list string
	at   time.Time
}

const echRetry = 10 * time.Minute

func dialOn(ctx context.Context, tr *quic.Transport, raddr net.Addr, tlsConf *tls.Config, conf *quic.Config) (*quic.Conn, error) {
	early := !prove.Swap(false)
	hidden := withECH(tlsConf)
	if hidden == nil {
		qc, err := dialVersions(ctx, tr, raddr, tlsConf, conf, early)
		if timedOut(err) && tlsConf != nil {
			noECH.Delete(tlsConf.ServerName)
		}
		return qc, err
	}
	qc, err := dialVersions(ctx, tr, raddr, hidden, conf, early)
	if err == nil || ctx.Err() != nil {
		return qc, err
	}
	fmt.Printf("dial     %s: ECH did not go through (%v), trying without\n", raddr, err)
	qc, err = dialVersions(ctx, tr, raddr, tlsConf, conf, early)
	if err == nil {
		noECH.Store(tlsConf.ServerName, echFailure{string(hidden.EncryptedClientHelloConfigList), time.Now()})
	}
	return qc, err
}

func withECH(tlsConf *tls.Config) *tls.Config {
	if ECH == nil || tlsConf == nil || tlsConf.ServerName == "" || tlsConf.EncryptedClientHelloConfigList != nil {
		return nil
	}
	list := ECH(tlsConf.ServerName)
	if len(list) == 0 {
		return nil
	}
	if held, ok := noECH.Load(tlsConf.ServerName); ok {
		if failed := held.(echFailure); failed.list == string(list) && time.Since(failed.at) < echRetry {
			return nil
		}
	}
	hidden := tlsConf.Clone()
	hidden.EncryptedClientHelloConfigList = list
	return hidden
}

func dialVersions(ctx context.Context, tr *quic.Transport, raddr net.Addr, tlsConf *tls.Config, conf *quic.Config, early bool) (*quic.Conn, error) {
	if skipV2.Load() {
		qc, err := dialVersion(ctx, tr, raddr, tlsConf, conf, quic.Version1, early)
		if err == nil && early {
			go watch(qc)
		}
		if timedOut(err) {
			skipV2.Store(false)
		}
		return qc, err
	}
	qc, err := dialVersion(ctx, tr, raddr, tlsConf, conf, quic.Version2, early)
	if err == nil {
		if early {
			go watch(qc)
		}
		return qc, nil
	}
	if !timedOut(err) || ctx.Err() != nil {
		return nil, err
	}
	fmt.Printf("dial     %s: no answer to QUIC v2, trying v1\n", raddr)
	qc, err = dialVersion(ctx, tr, raddr, tlsConf, conf, quic.Version1, early)
	if err == nil {
		skipV2.Store(true)
	}
	return qc, err
}

func dialVersion(ctx context.Context, tr *quic.Transport, raddr net.Addr, tlsConf *tls.Config, conf *quic.Config, v quic.Version, early bool) (*quic.Conn, error) {
	tlsConf = ensureALPN(tlsConf)
	conf = configOrDefault(conf).Clone()
	conf.Versions = []quic.Version{v}
	if v != quic.Version1 {
		conf.Versions = append(conf.Versions, quic.Version1)
	}
	if early && tlsConf.ClientSessionCache != nil {
		return tr.DialEarly(ctx, raddr, tlsConf, conf)
	}
	return tr.Dial(ctx, raddr, tlsConf, conf)
}

func watch(qc *quic.Conn) {
	select {
	case <-qc.HandshakeComplete():
	case <-qc.Context().Done():
		cause := context.Cause(qc.Context())
		var te *quic.TransportError
		if timedOut(cause) || errors.As(cause, &te) && te.ErrorCode.IsCryptoError() {
			fmt.Printf("dial     %s: the early handshake failed (%v), the next dial goes without 0-RTT to see whether QUIC v2 or ECH is at fault\n", qc.RemoteAddr(), cause)
			prove.Store(true)
		}
	}
}

func timedOut(err error) bool {
	var idle *quic.IdleTimeoutError
	var hs *quic.HandshakeTimeoutError
	return errors.As(err, &idle) || errors.As(err, &hs)
}

func Known(endpoint string) []netip.Addr {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{ip.Unmap()}
	}
	held, ok := known.Load(endpoint)
	if !ok {
		return nil
	}
	var out []netip.Addr
	for _, a := range held.([]*net.UDPAddr) {
		if ip, ok := netip.AddrFromSlice(a.IP); ok {
			out = append(out, ip.Unmap())
		}
	}
	return out
}

func refresh(endpoint, host string, port int) {
	look, stop := context.WithTimeout(context.Background(), resolveWait)
	defer stop()
	ips, err := net.DefaultResolver.LookupIP(look, "ip", host)
	if err != nil || len(ips) == 0 {
		return
	}
	known.Store(endpoint, order(ips, port))
}
