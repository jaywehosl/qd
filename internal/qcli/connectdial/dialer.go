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
	"time"

	"github.com/quic-go/quic-go/http3"
)

type Dialer struct {
	CC     *http3.ClientConn
	Header http.Header
}

func (d Dialer) DialTCP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	sctx, scancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	req := (&http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Scheme: "https", Host: dst.String()},
		Host:   dst.String(),
		Header: d.headers(),
		Body:   pr,
	}).WithContext(sctx)

	type result struct {
		resp *http.Response
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		resp, err := d.CC.RoundTrip(req)
		ch <- result{resp, err}
	}()

	select {
	case <-ctx.Done():
		scancel()
		pw.Close()
		return nil, fmt.Errorf("CONNECT %s: %w", dst, ctx.Err())
	case r := <-ch:
		if r.err != nil {
			scancel()
			pw.Close()
			return nil, fmt.Errorf("CONNECT %s: %w", dst, r.err)
		}
		if r.resp.StatusCode != http.StatusOK {
			scancel()
			pw.Close()
			r.resp.Body.Close()
			return nil, fmt.Errorf("CONNECT %s: статус %d", dst, r.resp.StatusCode)
		}
		return &streamConn{
			r:      r.resp.Body,
			w:      pw,
			cancel: scancel,
			remote: net.TCPAddrFromAddrPort(dst),
		}, nil
	}
}

func (d Dialer) DialUDP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	return nil, errors.New("connectdial: UDP идёт датаграммами, не CONNECT-стримом")
}

type streamConn struct {
	r      io.ReadCloser
	w      *io.PipeWriter
	cancel context.CancelFunc
	remote net.Addr
}

func (c *streamConn) Read(b []byte) (int, error)  { return c.r.Read(b) }
func (c *streamConn) Write(b []byte) (int, error) { return c.w.Write(b) }

func (c *streamConn) CloseWrite() error { return c.w.Close() }

func (c *streamConn) Close() error {
	c.w.Close()
	err := c.r.Close()
	c.cancel()
	return err
}

func (c *streamConn) LocalAddr() net.Addr  { return &net.TCPAddr{} }
func (c *streamConn) RemoteAddr() net.Addr { return c.remote }

func (c *streamConn) SetDeadline(time.Time) error      { return nil }
func (c *streamConn) SetReadDeadline(time.Time) error  { return nil }
func (c *streamConn) SetWriteDeadline(time.Time) error { return nil }

func (d Dialer) headers() http.Header {
	out := make(http.Header, len(d.Header))
	for k, v := range d.Header {
		out[k] = v
	}
	return out
}
