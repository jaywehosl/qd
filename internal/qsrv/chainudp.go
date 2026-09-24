package qsrv

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"github.com/jaywehosl/quic-diver/internal/costream"
)

const chainAnswerWait = 15 * time.Second

func (c chained) DialUDP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	open, cancel := context.WithTimeout(ctx, chainAnswerWait)
	defer cancel()

	rs, err := c.cc.OpenRequestStream(open)
	if err != nil {
		return nil, err
	}
	abort := func() {
		rs.CancelRead(0)
		rs.CancelWrite(0)
	}

	head := http.Header{}
	head.Set(HeaderProto, "udp")
	head.Set(HeaderDgram, "1")
	head.Set(HeaderHops, strconv.Itoa(c.hops))
	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Scheme: "https", Host: dst.String()},
		Host:   dst.String(),
		Header: head,
	}
	if err := rs.SendRequestHeader(req); err != nil {
		abort()
		return nil, err
	}

	stop := context.AfterFunc(open, abort)
	rsp, err := rs.ReadResponse()
	if !stop() {
		return nil, fmt.Errorf("%s did not take the flow: %w", c.endpoint, open.Err())
	}
	if err != nil {
		abort()
		return nil, err
	}
	if rsp.StatusCode != http.StatusOK {
		abort()
		return nil, fmt.Errorf("%s refused the flow: %s", c.endpoint, rsp.Status)
	}

	at := c.at()
	c.ls.hold(at)
	done := func() { c.ls.release(at) }
	if rsp.Header.Get(HeaderDgram) != "1" {
		return costream.NewPackets(rs, rs, abort, dst, done), nil
	}
	return newDgramConn(rs, dst, abort, done), nil
}

type dgramConn struct {
	rs   *http3.RequestStream
	dst  netip.AddrPort
	in   chan []byte
	quit chan struct{}
	once sync.Once
	wmu  sync.Mutex

	abort func()
	done  func()
}

func newDgramConn(rs *http3.RequestStream, dst netip.AddrPort, abort, done func()) *dgramConn {
	d := &dgramConn{rs: rs, dst: dst, in: make(chan []byte, 512), quit: make(chan struct{}), abort: abort, done: done}
	go d.fromDatagrams()
	go d.fromStream()
	return d
}

func (d *dgramConn) fromDatagrams() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-d.quit
		cancel()
	}()
	for {
		b, err := d.rs.ReceiveDatagram(ctx)
		if err != nil {
			d.Close()
			return
		}
		d.deliver(b)
	}
}

func (d *dgramConn) fromStream() {
	for {
		b, err := readFrame(d.rs)
		if err != nil {
			d.Close()
			return
		}
		d.deliver(b)
	}
}

func (d *dgramConn) deliver(b []byte) {
	select {
	case d.in <- b:
	case <-d.quit:
	default:
	}
}

func (d *dgramConn) Read(p []byte) (int, error) {
	select {
	case b := <-d.in:
		return copy(p, b), nil
	case <-d.quit:
		return 0, net.ErrClosed
	}
}

func (d *dgramConn) Write(p []byte) (int, error) {
	err := d.rs.SendDatagram(p)
	if errors.Is(err, &quic.DatagramTooLargeError{}) {
		d.wmu.Lock()
		_, err = d.rs.Write(frame(p))
		d.wmu.Unlock()
	}
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (d *dgramConn) Close() error {
	d.once.Do(func() {
		close(d.quit)
		d.abort()
		if d.done != nil {
			d.done()
		}
	})
	return nil
}

func (d *dgramConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (d *dgramConn) RemoteAddr() net.Addr             { return net.UDPAddrFromAddrPort(d.dst) }
func (d *dgramConn) SetDeadline(time.Time) error      { return nil }
func (d *dgramConn) SetReadDeadline(time.Time) error  { return nil }
func (d *dgramConn) SetWriteDeadline(time.Time) error { return nil }

func frame(p []byte) []byte {
	out := make([]byte, 2+len(p))
	binary.BigEndian.PutUint16(out, uint16(len(p)))
	copy(out[2:], p)
	return out
}

func readFrame(r io.Reader) ([]byte, error) {
	var size [2]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return nil, err
	}
	b := make([]byte, binary.BigEndian.Uint16(size[:]))
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (n *Node) relayDatagrams(w http.ResponseWriter, hs http3.HTTPStreamer, out net.Conn, s *live) {
	defer out.Close()

	w.Header().Set(HeaderDgram, "1")
	w.WriteHeader(http.StatusOK)
	str := hs.HTTPStream()
	defer func() {
		str.CancelRead(0)
		str.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var lastOut atomic.Int64
	lastOut.Store(time.Now().UnixNano())
	up := func(b []byte) bool {
		if _, err := out.Write(b); err != nil {
			return false
		}
		lastOut.Store(time.Now().UnixNano())
		if s != nil {
			s.wentUp(len(b))
		}
		return true
	}
	go func() {
		defer out.Close()
		for {
			b, err := str.ReceiveDatagram(ctx)
			if err != nil || !up(b) {
				return
			}
		}
	}()
	go func() {
		defer out.Close()
		for {
			b, err := readFrame(str)
			if err != nil || !up(b) {
				return
			}
		}
	}()

	buf := make([]byte, 65535)
	for {
		out.SetReadDeadline(time.Now().Add(flowQuiet))
		read, err := out.Read(buf)
		if err != nil {
			if quiet, ok := err.(net.Error); ok && quiet.Timeout() &&
				time.Since(time.Unix(0, lastOut.Load())) < flowQuiet {
				continue
			}
			return
		}
		err = str.SendDatagram(buf[:read])
		if errors.Is(err, &quic.DatagramTooLargeError{}) {
			_, err = str.Write(frame(buf[:read]))
		}
		if err != nil {
			return
		}
		if s != nil {
			s.cameDown(read)
		}
	}
}
