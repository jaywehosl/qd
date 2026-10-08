package costream

import (
	"context"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"
)

type shared struct {
	r      io.ReadCloser
	w      io.WriteCloser
	cancel context.CancelFunc
	done   func()
	once   sync.Once
}

func (s *shared) shut() error {
	s.w.Close()
	err := s.r.Close()
	if s.cancel != nil {
		s.cancel()
	}
	if s.done != nil {
		s.once.Do(s.done)
	}
	return err
}

type Stream struct {
	shared
	dst netip.AddrPort
}

func NewStream(r io.ReadCloser, w io.WriteCloser, cancel context.CancelFunc, dst netip.AddrPort, done func()) *Stream {
	return &Stream{shared: shared{r: r, w: w, cancel: cancel, done: done}, dst: dst}
}

func (s *Stream) Read(b []byte) (int, error)  { return s.r.Read(b) }
func (s *Stream) Write(b []byte) (int, error) { return s.w.Write(b) }

func (s *Stream) CloseWrite() error { return s.w.Close() }

func (s *Stream) Close() error { return s.shut() }

func (s *Stream) LocalAddr() net.Addr  { return &net.TCPAddr{} }
func (s *Stream) RemoteAddr() net.Addr { return net.TCPAddrFromAddrPort(s.dst) }

func (s *Stream) SetDeadline(time.Time) error      { return nil }
func (s *Stream) SetReadDeadline(time.Time) error  { return nil }
func (s *Stream) SetWriteDeadline(time.Time) error { return nil }

func NewReply() *Reply {
	return &Reply{ready: make(chan struct{})}
}

type Reply struct {
	mu     sync.Mutex
	ready  chan struct{}
	body   io.ReadCloser
	err    error
	closed bool
}

func (r *Reply) Settle(body io.ReadCloser, err error) {
	r.mu.Lock()
	r.body, r.err = body, err
	late := r.closed
	close(r.ready)
	r.mu.Unlock()
	if late && body != nil {
		body.Close()
	}
}

func (r *Reply) Read(b []byte) (int, error) {
	<-r.ready
	if r.err != nil {
		return 0, r.err
	}
	return r.body.Read(b)
}

func (r *Reply) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	select {
	case <-r.ready:
		if r.body != nil {
			return r.body.Close()
		}
	default:
	}
	return nil
}
