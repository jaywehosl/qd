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
	"syscall"
	"time"

	quic "github.com/quic-go/quic-go"

	"github.com/jaywehosl/qd/internal/pace"
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

func (c *Conn) Reaches(ctx context.Context) error {
	remote, ok := c.remote.(*net.UDPAddr)
	if !ok {
		return nil
	}
	d := net.Dialer{}
	if c.keep != nil {
		d.Control = func(_, _ string, rc syscall.RawConn) error { return rc.Control(c.keep) }
	}
	probe, err := d.DialContext(ctx, "udp", remote.String())
	if err != nil {
		return fmt.Errorf("this network has no way to %s: %w", remote.IP, err)
	}
	return probe.Close()
}

func (c *Conn) Migrate(ctx context.Context, laddr *net.UDPAddr) error {
	if err := c.Reaches(ctx); err != nil {
		return err
	}
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
	TLS   *tls.Config
	QUIC  *quic.Config
	Keep  func(fd uintptr)
	Prove func(ctx context.Context, c *Conn, first bool) error
}

type candidate struct {
	addr  *net.UDPAddr
	v     quic.Version
	plain bool
	after time.Duration
}

type proof struct {
	addr string
	v    quic.Version
}

var best sync.Map

var OnlyV4 atomic.Bool

func Forget() {
	best.Clear()
	noECH.Clear()
}

func (d Dialer) Dial(ctx context.Context, endpoint string) (*Conn, error) {
	addrs, err := resolve(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	lane := d.lane(endpoint, addrs)

	round, stop := context.WithCancel(ctx)
	defer stop()

	type finish struct {
		conn *Conn
		who  candidate
		err  error
	}
	line := make(chan finish, len(lane))
	run := func(n int) {
		who := lane[n]
		conn, err := d.open(round, who)
		if err == nil && d.Prove != nil {
			if err = d.Prove(round, conn, n == 0); err != nil {
				conn.Close()
			}
		}
		if err != nil {
			line <- finish{who: who, err: fmt.Errorf("%s: %w", who.addr, err)}
			return
		}
		line <- finish{conn: conn, who: who}
	}
	drain := func(left int) {
		for ; left > 0; left-- {
			if late := <-line; late.conn != nil {
				late.conn.Close()
			}
		}
	}

	began := time.Now()
	next := time.NewTimer(0)
	defer next.Stop()
	started, ended, refused := 0, 0, false
	tried := make(failures, 0, len(lane))
	for {
		select {
		case <-next.C:
			if started == len(lane) {
				continue
			}
			if who := lane[started]; who.after > 0 {
				early := refused
				if !who.plain {
					early = ended == started
				}
				if held := time.Until(began.Add(who.after)); held > 0 && !early {
					next.Reset(held)
					continue
				}
			}
			go run(started)
			started++
			next.Reset(pace.Stagger)
		case got := <-line:
			ended++
			if got.err == nil {
				go drain(started - ended)
				best.Store(endpoint, proof{got.who.addr.String(), got.who.v})
				if hidden := withECH(d.TLS); got.who.plain && hidden != nil {
					NoteECH(d.TLS, hidden)
				}
				fmt.Printf("dial     %s answered in %d ms over QUIC %s\n", got.who.addr, time.Since(began).Milliseconds(), got.who.v)
				return got.conn, nil
			}
			var rejected *tls.ECHRejectionError
			refused = refused || errors.As(got.err, &rejected)
			tried = append(tried, got.err)
			if ended == len(lane) || errors.Is(got.err, quic.Err0RTTRejected) {
				go drain(started - ended)
				return nil, tried
			}
			if ended == started {
				next.Reset(0)
			}
		case <-ctx.Done():
			go drain(started - ended)
			return nil, ctx.Err()
		}
	}
}

type failures []error

func (f failures) Error() string {
	said := make([]string, len(f))
	for i, one := range f {
		said[i] = one.Error()
	}
	return strings.Join(said, " / ")
}

func (f failures) Unwrap() []error { return f }

func (d Dialer) lane(endpoint string, addrs []*net.UDPAddr) []candidate {
	versions := []quic.Version{quic.Version2, quic.Version1}
	out := make([]candidate, 0, 3*len(addrs))
	if held, ok := best.Load(endpoint); ok {
		was := held.(proof)
		if was.v == quic.Version1 {
			versions[0], versions[1] = versions[1], versions[0]
		}
		for _, a := range addrs {
			if a.String() == was.addr {
				out = append(out, candidate{addr: a, v: was.v})
			}
		}
	}
	for _, v := range versions {
		for _, a := range addrs {
			if len(out) > 0 && a == out[0].addr && v == out[0].v {
				continue
			}
			out = append(out, candidate{addr: a, v: v})
		}
	}
	if withECH(d.TLS) != nil {
		for _, a := range addrs {
			out = append(out, candidate{addr: a, v: versions[0], plain: true, after: pace.QUICHeadStart})
		}
	}
	return out
}

func (d Dialer) open(ctx context.Context, who candidate) (*Conn, error) {
	pc, err := listenFor(who.addr)
	if err != nil {
		fmt.Printf("dial     %s: no socket: %v\n", who.addr, err)
		return nil, err
	}
	setUDPBuffers(pc)
	keepOutside(pc, d.Keep)
	fmt.Printf("dial     %s from %s\n", who.addr, pc.LocalAddr())
	tr := &quic.Transport{Conn: pc}

	conf := d.TLS
	if hidden := withECH(d.TLS); hidden != nil && !who.plain {
		conf = hidden
	}
	began := time.Now()
	qc, err := dialVersion(ctx, tr, who.addr, conf, d.QUIC, who.v, d.Prove != nil)
	if err != nil {
		if ctx.Err() == nil {
			fmt.Printf("dial     %s gave up after %d ms: %v\n", who.addr, time.Since(began).Milliseconds(), err)
		}
		tr.Close()
		pc.Close()
		return nil, err
	}
	c := &Conn{qc: qc, tr: tr, pc: pc, remote: who.addr, keep: d.Keep}
	c.maxDgram.Store(defaultMaxDatagram)
	return c, nil
}

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
	conf := tlsConf
	if hidden := withECH(tlsConf); hidden != nil {
		conf = hidden
	}
	qc, err := dialVersion(ctx, tr, raddr, conf, quicConf, quic.Version2, true)
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
	if len(out) == 0 {
		return nil, fmt.Errorf("no ipv4 address for %s", host)
	}
	known.Store(endpoint, out)
	fmt.Printf("resolve  %s -> %v (v6 route %v)\n", endpoint, out, holdsV6())
	return out, nil
}

func order(ips []net.IP, port int) []*net.UDPAddr {
	first := make([]*net.UDPAddr, 0, len(ips))
	rest := make([]*net.UDPAddr, 0, len(ips))
	for _, ip := range ips {
		one := &net.UDPAddr{IP: ip, Port: port}
		if ip.To4() != nil {
			first = append(first, one)
			continue
		}
		rest = append(rest, one)
	}
	if OnlyV4.Load() {
		return first
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
	ECH   func(serverName string) []byte
	noECH sync.Map
)

type echFailure struct {
	list string
	at   time.Time
}

const echRetry = 10 * time.Minute

func Hidden(tlsConf *tls.Config) *tls.Config { return withECH(tlsConf) }

func NoteECH(plain, hidden *tls.Config) {
	noECH.Store(plain.ServerName, echFailure{string(hidden.EncryptedClientHelloConfigList), time.Now()})
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
	if fresh := order(ips, port); len(fresh) > 0 {
		known.Store(endpoint, fresh)
	}
}
