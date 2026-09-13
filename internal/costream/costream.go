package costream

import (
	"context"
	"encoding/binary"
	"fmt"
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

type Packets struct {
	shared
	dst netip.AddrPort

	writing sync.Mutex
	head    [2]byte
}

func NewPackets(r io.ReadCloser, w io.WriteCloser, cancel context.CancelFunc, dst netip.AddrPort, done func()) *Packets {
	return &Packets{shared: shared{r: r, w: w, cancel: cancel, done: done}, dst: dst}
}

func (p *Packets) Read(b []byte) (int, error) {
	var size [2]byte
	if _, err := io.ReadFull(p.r, size[:]); err != nil {
		return 0, err
	}
	want := int(binary.BigEndian.Uint16(size[:]))
	if want > len(b) {
		if _, err := io.CopyN(io.Discard, p.r, int64(want)); err != nil {
			return 0, err
		}
		return 0, nil
	}
	return io.ReadFull(p.r, b[:want])
}

func (p *Packets) Write(b []byte) (int, error) {
	if len(b) > 65535 {
		return 0, fmt.Errorf("datagram too large: %d", len(b))
	}
	p.writing.Lock()
	defer p.writing.Unlock()

	binary.BigEndian.PutUint16(p.head[:], uint16(len(b)))
	if _, err := p.w.Write(p.head[:]); err != nil {
		return 0, err
	}
	if _, err := p.w.Write(b); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (p *Packets) Close() error { return p.shut() }

func (p *Packets) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (p *Packets) RemoteAddr() net.Addr             { return net.UDPAddrFromAddrPort(p.dst) }
func (p *Packets) SetDeadline(time.Time) error      { return nil }
func (p *Packets) SetReadDeadline(time.Time) error  { return nil }
func (p *Packets) SetWriteDeadline(time.Time) error { return nil }
