package quicconn

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	quic "github.com/quic-go/quic-go"

	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink"
)

const ALPN = "qd/1"

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

func (c *Conn) RecvDatagram(ctx context.Context) ([]byte, error) {
	return c.qc.ReceiveDatagram(ctx)
}

func (c *Conn) OpenStream(ctx context.Context) (uplink.Stream, error) {
	s, err := c.qc.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (c *Conn) MaxDatagramSize() int {
	if v := c.maxDgram.Load(); v > 0 {
		return int(v)
	}
	return defaultMaxDatagram
}

func (c *Conn) QUIC() *quic.Conn { return c.qc }

func (c *Conn) Migrate(ctx context.Context, laddr *net.UDPAddr) error {
	pc, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return err
	}
	setUDPBuffers(pc)
	newTr := &quic.Transport{Conn: pc}

	path, err := c.qc.AddPath(newTr)
	if err != nil {
		newTr.Close()
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
	prev := c.prev
	c.prev = nil
	c.mu.Unlock()
	if tr != nil {
		tr.Close()
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

var _ uplink.Conn = (*Conn)(nil)

type Dialer struct {
	TLS  *tls.Config
	QUIC *quic.Config
	Keep func(fd uintptr)
}

func (d Dialer) Dial(ctx context.Context, endpoint string) (uplink.Conn, error) {
	raddr, err := resolve(endpoint)
	if err != nil {
		return nil, err
	}
	pc, err := net.ListenUDP("udp", &net.UDPAddr{Port: 0})
	if err != nil {
		return nil, err
	}
	setUDPBuffers(pc)
	keepOutside(pc, d.Keep)
	tr := &quic.Transport{Conn: pc}

	qc, err := tr.Dial(ctx, raddr, ensureALPN(d.TLS), configOrDefault(d.QUIC))
	if err != nil {
		tr.Close()
		return nil, err
	}
	c := &Conn{qc: qc, tr: tr, pc: pc, remote: raddr}
	c.maxDgram.Store(defaultMaxDatagram)
	return c, nil
}

var _ uplink.Dialer = Dialer{}

func DefaultConfig() *quic.Config {
	return &quic.Config{
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
		t.NextProtos = []string{ALPN}
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

func resolve(endpoint string) (*net.UDPAddr, error) {
	addr, err := net.ResolveUDPAddr("udp", endpoint)
	if err == nil {
		known.Store(endpoint, addr)
		return addr, nil
	}
	if held, ok := known.Load(endpoint); ok {
		return held.(*net.UDPAddr), nil
	}
	return nil, err
}
