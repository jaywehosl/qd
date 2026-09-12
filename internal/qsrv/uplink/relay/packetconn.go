package relay

import (
	"net"
	"os"
	"sync"
	"time"
)

type addr struct{}

func (addr) Network() string { return "relay" }
func (addr) String() string  { return "relay" }

var Peer net.Addr = addr{}

type PacketConn struct {
	s      *Session
	in     chan []byte
	rd     *deadline
	wd     *deadline
	closed chan struct{}
	once   sync.Once
}

func NewPacketConn(s *Session) *PacketConn {
	pc := &PacketConn{
		s:      s,
		in:     make(chan []byte, 1024),
		rd:     newDeadline(),
		wd:     newDeadline(),
		closed: make(chan struct{}),
	}
	s.OnReceive(func(b []byte) {
		select {
		case pc.in <- append([]byte(nil), b...):
		case <-pc.closed:
		default:
		}
	})
	return pc
}

func (pc *PacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	select {
	case <-pc.closed:
		return 0, nil, net.ErrClosed
	default:
	}
	select {
	case b := <-pc.in:
		return copy(p, b), Peer, nil
	case <-pc.rd.wait():
		return 0, nil, os.ErrDeadlineExceeded
	case <-pc.closed:
		return 0, nil, net.ErrClosed
	}
}

func (pc *PacketConn) WriteTo(p []byte, _ net.Addr) (int, error) {
	select {
	case <-pc.closed:
		return 0, net.ErrClosed
	default:
	}
	_ = pc.s.Send(p)
	return len(p), nil
}

func (pc *PacketConn) Close() error {
	pc.once.Do(func() { close(pc.closed) })
	return pc.s.Stop()
}

func (pc *PacketConn) LocalAddr() net.Addr { return Peer }

func (pc *PacketConn) SetDeadline(t time.Time) error {
	pc.rd.set(t)
	pc.wd.set(t)
	return nil
}

func (pc *PacketConn) SetReadDeadline(t time.Time) error {
	pc.rd.set(t)
	return nil
}

func (pc *PacketConn) SetWriteDeadline(t time.Time) error {
	pc.wd.set(t)
	return nil
}

type deadline struct {
	mu     sync.Mutex
	timer  *time.Timer
	cancel chan struct{}
}

func newDeadline() *deadline {
	return &deadline{cancel: make(chan struct{})}
}

func (d *deadline) set(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil && !d.timer.Stop() {
		<-d.cancel
	}
	d.timer = nil

	closed := false
	select {
	case <-d.cancel:
		closed = true
	default:
	}

	if t.IsZero() {
		if closed {
			d.cancel = make(chan struct{})
		}
		return
	}

	if dur := time.Until(t); dur > 0 {
		if closed {
			d.cancel = make(chan struct{})
		}
		d.timer = time.AfterFunc(dur, func() { close(d.cancel) })
		return
	}

	if !closed {
		close(d.cancel)
	}
}

func (d *deadline) wait() chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cancel
}

var _ net.PacketConn = (*PacketConn)(nil)
