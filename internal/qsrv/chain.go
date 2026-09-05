package qsrv

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/quic-go/quic-go/http3"
)

type chained struct {
	cc       *http3.ClientConn
	ls       *links
	endpoint string
	seat     uint32
	route    string
	hops     int
}

func (c chained) key() string { return seatKey(c.endpoint, c.seat) }

func (c chained) DialTCP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	sctx, scancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()

	req := (&http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Scheme: "https", Host: dst.String()},
		Host:   dst.String(),
		Header: http.Header{},
		Body:   pr,
	}).WithContext(sctx)
	if c.route != "" {
		req.Header.Set(HeaderRoute, c.route)
	}
	req.Header.Set(HeaderHops, strconv.Itoa(c.hops))

	type answer struct {
		rsp *http.Response
		err error
	}
	done := make(chan answer, 1)
	go func() {
		rsp, err := c.cc.RoundTrip(req)
		done <- answer{rsp, err}
	}()

	select {
	case <-ctx.Done():
		scancel()
		pw.Close()
		return nil, fmt.Errorf("%s did not take the flow: %w", c.endpoint, ctx.Err())
	case got := <-done:
		if got.err != nil {
			scancel()
			pw.Close()
			return nil, got.err
		}
		if got.rsp.StatusCode != http.StatusOK {
			got.rsp.Body.Close()
			scancel()
			pw.Close()
			return nil, fmt.Errorf("%s refused the flow: %s", c.endpoint, got.rsp.Status)
		}
		key := c.key()
		c.ls.hold(key)
		return &streamConn{r: got.rsp.Body, w: pw, dst: dst, stop: scancel,
			done: func() { c.ls.release(key) }}, nil
	}
}

type streamConn struct {
	r    io.ReadCloser
	w    io.WriteCloser
	dst  netip.AddrPort
	stop context.CancelFunc
	done func()
	once sync.Once
}

func (s *streamConn) Read(p []byte) (int, error)  { return s.r.Read(p) }
func (s *streamConn) Write(p []byte) (int, error) { return s.w.Write(p) }

func (s *streamConn) Close() error {
	s.w.Close()
	err := s.r.Close()
	if s.stop != nil {
		s.stop()
	}
	if s.done != nil {
		s.once.Do(s.done)
	}
	return err
}

func (s *streamConn) CloseWrite() error { return s.w.Close() }

func (s *streamConn) LocalAddr() net.Addr  { return &net.TCPAddr{} }
func (s *streamConn) RemoteAddr() net.Addr { return net.TCPAddrFromAddrPort(s.dst) }

func (s *streamConn) SetDeadline(time.Time) error      { return nil }
func (s *streamConn) SetReadDeadline(time.Time) error  { return nil }
func (s *streamConn) SetWriteDeadline(time.Time) error { return nil }

func (c chained) DialUDP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	sctx, scancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()

	req := (&http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Scheme: "https", Host: dst.String()},
		Host:   dst.String(),
		Header: http.Header{},
		Body:   pr,
	}).WithContext(sctx)
	req.Header.Set(HeaderProto, "udp")
	if c.route != "" {
		req.Header.Set(HeaderRoute, c.route)
	}
	req.Header.Set(HeaderHops, strconv.Itoa(c.hops))

	type answer struct {
		rsp *http.Response
		err error
	}
	done := make(chan answer, 1)
	go func() {
		rsp, err := c.cc.RoundTrip(req)
		done <- answer{rsp, err}
	}()

	select {
	case <-ctx.Done():
		scancel()
		pw.Close()
		return nil, fmt.Errorf("%s did not take the flow: %w", c.endpoint, ctx.Err())
	case got := <-done:
		if got.err != nil {
			scancel()
			pw.Close()
			return nil, got.err
		}
		if got.rsp.StatusCode != http.StatusOK {
			got.rsp.Body.Close()
			scancel()
			pw.Close()
			return nil, fmt.Errorf("%s refused the flow: %s", c.endpoint, got.rsp.Status)
		}
		key := c.key()
		c.ls.hold(key)
		return &packetConn{r: got.rsp.Body, w: pw, dst: dst, stop: scancel,
			done: func() { c.ls.release(key) }}, nil
	}
}

type packetConn struct {
	r    io.ReadCloser
	w    io.WriteCloser
	dst  netip.AddrPort
	stop context.CancelFunc
	done func()
	once sync.Once

	writing sync.Mutex
	head    [2]byte
}

func (p *packetConn) Read(b []byte) (int, error) {
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

func (p *packetConn) Write(b []byte) (int, error) {
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

func (p *packetConn) Close() error {
	p.w.Close()
	err := p.r.Close()
	if p.stop != nil {
		p.stop()
	}
	return err
}

func (p *packetConn) LocalAddr() net.Addr  { return &net.UDPAddr{} }
func (p *packetConn) RemoteAddr() net.Addr { return net.UDPAddrFromAddrPort(p.dst) }

func (p *packetConn) SetDeadline(time.Time) error      { return nil }
func (p *packetConn) SetReadDeadline(time.Time) error  { return nil }
func (p *packetConn) SetWriteDeadline(time.Time) error { return nil }
