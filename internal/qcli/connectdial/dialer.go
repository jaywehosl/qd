// Package connectdial — исходящие TCP-соединения клиента через надёжный
// QUIC-стрим (HTTP/3 CONNECT, RFC 9114) до узла.
//
// Это клиентская половина гибрида: TCP-флоу терминируется локальным gVisor и
// уезжает в CONNECT-стрим, где потери туннеля закрывает ретрансмит QUIC —
// внутренний TCP приложения их не видит (в отличие от датаграмм connect-ip).
// UDP по-прежнему идёт датаграммами: там ретрансмит только навредил бы.
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
	"time"

	"github.com/quic-go/quic-go/http3"
)

// Dialer открывает CONNECT-стримы через существующее H3-соединение с узлом.
type Dialer struct {
	CC     *http3.ClientConn
	Header http.Header
}

// DialTCP просит узел соединиться с dst и отдаёт поток как net.Conn.
//
// ВАЖНО: стрим живёт ровно столько, сколько живёт контекст запроса, поэтому
// переданный ctx НЕЛЬЗЯ отдавать в запрос — вызывающий (netstack.handleTCP)
// отменяет его сразу после дозвона (что верно для net.Dial, но убило бы стрим).
// Здесь ctx ограничивает только фазу дозвона, а стрим завязан на собственный
// контекст, который отменяется при Close.
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
	case <-ctx.Done(): // не уложились в дозвон
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

// DialUDP не поддерживается: UDP в гибриде идёт датаграммами connect-ip, а не
// стримами (ретрансмит для UDP вреден).
func (d Dialer) DialUDP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	return nil, errors.New("connectdial: UDP идёт датаграммами, не CONNECT-стримом")
}

// streamConn — net.Conn поверх CONNECT-стрима (чтение из тела ответа, запись в
// тело запроса).
type streamConn struct {
	r      io.ReadCloser
	w      *io.PipeWriter
	cancel context.CancelFunc // отменяет контекст стрима при Close
	remote net.Addr
}

func (c *streamConn) Read(b []byte) (int, error)  { return c.r.Read(b) }
func (c *streamConn) Write(b []byte) (int, error) { return c.w.Write(b) }

// CloseWrite закрывает только сторону записи (полу-закрытие TCP): узел увидит
// EOF и дошлёт остаток ответа.
func (c *streamConn) CloseWrite() error { return c.w.Close() }

func (c *streamConn) Close() error {
	c.w.Close()
	err := c.r.Close()
	c.cancel() // теперь стрим можно гасить
	return err
}

func (c *streamConn) LocalAddr() net.Addr  { return &net.TCPAddr{} }
func (c *streamConn) RemoteAddr() net.Addr { return c.remote }

// Дедлайны на CONNECT-стриме не нужны: временем управляет ctx запроса и
// закрытие соединения.
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
