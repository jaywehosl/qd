package connectdial

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"

	"github.com/quic-go/quic-go/http3"
	"golang.org/x/net/http2"

	"github.com/jaywehosl/quic-diver/internal/costream"
	"github.com/jaywehosl/quic-diver/internal/qsrv"
)

type Dialer struct {
	CC     *http3.ClientConn
	H2     *http2.ClientConn
	Header http.Header
}

func (d Dialer) Packets() bool { return d.CC == nil && d.H2 != nil }

func (d Dialer) roundTrip(ctx context.Context, req *http.Request) (*http.Response, error) {
	type answer struct {
		rsp *http.Response
		err error
	}
	done := make(chan answer, 1)

	go func() {
		if d.CC != nil {
			rsp, err := d.CC.RoundTrip(req)
			done <- answer{rsp, err}
			return
		}
		rsp, err := d.H2.RoundTrip(req)
		done <- answer{rsp, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case got := <-done:
		return got.rsp, got.err
	}
}

func (d Dialer) open(ctx context.Context, dst netip.AddrPort, udp bool) (io.ReadCloser, io.WriteCloser, context.CancelFunc, error) {
	sctx, scancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()

	head := d.Header.Clone()
	if head == nil {
		head = http.Header{}
	}
	if udp {
		head.Set(qsrv.HeaderProto, "udp")
	}

	req := (&http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Scheme: "https", Host: dst.String()},
		Host:   dst.String(),
		Header: head,
		Body:   pr,
	}).WithContext(sctx)

	give := func(why error) (io.ReadCloser, io.WriteCloser, context.CancelFunc, error) {
		scancel()
		pw.Close()
		return nil, nil, nil, why
	}

	rsp, err := d.roundTrip(ctx, req)
	if err != nil {
		return give(fmt.Errorf("CONNECT %s: %w", dst, err))
	}
	if rsp.StatusCode != http.StatusOK {
		rsp.Body.Close()
		return give(fmt.Errorf("CONNECT %s: status %d", dst, rsp.StatusCode))
	}
	return rsp.Body, pw, scancel, nil
}

func (d Dialer) DialTCP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	r, w, stop, err := d.open(ctx, dst, false)
	if err != nil {
		return nil, err
	}
	return costream.NewStream(r, w, stop, dst, nil), nil
}

func (d Dialer) DialUDP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	if !d.Packets() {
		return nil, errors.New("connectdial: UDP travels as datagrams, not as a CONNECT stream")
	}
	r, w, stop, err := d.open(ctx, dst, true)
	if err != nil {
		return nil, err
	}
	return costream.NewPackets(r, w, stop, dst, nil), nil
}
