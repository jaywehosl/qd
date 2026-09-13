package qsrv

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"

	"github.com/quic-go/quic-go/http3"

	"github.com/jaywehosl/quic-diver/internal/costream"
)

type chained struct {
	cc       *http3.ClientConn
	ls       *links
	endpoint string
	seat     uint32
	hops     int
}

func (c chained) at() where { return where{c.endpoint, c.seat} }

func (c chained) open(ctx context.Context, dst netip.AddrPort, udp bool) (io.ReadCloser, io.WriteCloser, context.CancelFunc, func(), error) {
	sctx, scancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()

	head := http.Header{}
	if udp {
		head.Set(HeaderProto, "udp")
	}
	head.Set(HeaderHops, strconv.Itoa(c.hops))

	req := (&http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Scheme: "https", Host: dst.String()},
		Host:   dst.String(),
		Header: head,
		Body:   pr,
	}).WithContext(sctx)

	type answer struct {
		rsp *http.Response
		err error
	}
	done := make(chan answer, 1)
	go func() {
		rsp, err := c.cc.RoundTrip(req)
		done <- answer{rsp, err}
	}()

	give := func(why error) (io.ReadCloser, io.WriteCloser, context.CancelFunc, func(), error) {
		scancel()
		pw.Close()
		return nil, nil, nil, nil, why
	}

	select {
	case <-ctx.Done():
		return give(fmt.Errorf("%s did not take the flow: %w", c.endpoint, ctx.Err()))
	case got := <-done:
		if got.err != nil {
			return give(got.err)
		}
		if got.rsp.StatusCode != http.StatusOK {
			got.rsp.Body.Close()
			return give(fmt.Errorf("%s refused the flow: %s", c.endpoint, got.rsp.Status))
		}
		at := c.at()
		c.ls.hold(at)
		return got.rsp.Body, pw, scancel, func() { c.ls.release(at) }, nil
	}
}

func (c chained) DialTCP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	r, w, stop, done, err := c.open(ctx, dst, false)
	if err != nil {
		return nil, err
	}
	return costream.NewStream(r, w, stop, dst, done), nil
}

func (c chained) DialUDP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	r, w, stop, done, err := c.open(ctx, dst, true)
	if err != nil {
		return nil, err
	}
	return costream.NewPackets(r, w, stop, dst, done), nil
}
