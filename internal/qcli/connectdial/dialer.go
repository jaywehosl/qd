// Package connectdial — исходящие соединения клиента через надёжный стрим
// (HTTP CONNECT, RFC 9114 / RFC 9113) до узла.
//
// Это клиентская половина гибрида: TCP-флоу терминируется локальным gVisor и
// уезжает в CONNECT-стрим, где потери туннеля закрывает ретрансмит транспорта —
// внутренний TCP приложения их не видит (в отличие от датаграмм connect-ip).
//
// Реализует netstack.Dialer, поэтому серверный forwarder-код переиспользуется
// на клиенте без изменений — меняется только способ выхода наружу.
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

// Dialer открывает CONNECT-стримы через существующее соединение с узлом.
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

// open просит узел соединиться с dst и отдаёт половинки стрима.
//
// ВАЖНО: стрим живёт ровно столько, сколько живёт контекст запроса, поэтому
// переданный ctx НЕЛЬЗЯ отдавать в запрос — вызывающий (netstack.handleTCP)
// отменяет его сразу после дозвона (что верно для net.Dial, но убило бы стрим).
// Здесь ctx ограничивает только фазу дозвона, а стрим завязан на собственный
// контекст, который отменяется при Close.
func (d Dialer) open(ctx context.Context, dst netip.AddrPort, udp bool) (io.ReadCloser, io.WriteCloser, context.CancelFunc, error) {
	sctx, scancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()

	head := d.headers()
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
		return give(fmt.Errorf("CONNECT %s: статус %d", dst, rsp.StatusCode))
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
		return nil, errors.New("connectdial: UDP идёт датаграммами, не CONNECT-стримом")
	}
	r, w, stop, err := d.open(ctx, dst, true)
	if err != nil {
		return nil, err
	}
	return costream.NewPackets(r, w, stop, dst, nil), nil
}

func (d Dialer) headers() http.Header {
	out := make(http.Header, len(d.Header))
	for k, v := range d.Header {
		out[k] = v
	}
	return out
}
