package costream

import (
	"context"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"
)

const (
	relaySmall = 32 << 10
	relayLarge = 128 << 10
)

var (
	smallBufs = sync.Pool{New: func() any { return new([relaySmall]byte) }}
	largeBufs = sync.Pool{New: func() any { return new([relayLarge]byte) }}
)

func Relay(dst io.Writer, src io.Reader) (int64, error) {
	small := smallBufs.Get().(*[relaySmall]byte)
	buf := small[:]
	var large *[relayLarge]byte
	defer func() {
		smallBufs.Put(small)
		if large != nil {
			largeBufs.Put(large)
		}
	}()
	var total int64
	full := 0
	for {
		n, err := src.Read(buf)
		if n > 0 {
			wrote, werr := dst.Write(buf[:n])
			total += int64(wrote)
			if werr != nil {
				return total, werr
			}
			if wrote < n {
				return total, io.ErrShortWrite
			}
		}
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return total, err
		}
		if large == nil {
			if n == len(buf) {
				full++
			} else {
				full = 0
			}
			if full >= 2 {
				large = largeBufs.Get().(*[relayLarge]byte)
				buf = large[:]
			}
		}
	}
}

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
