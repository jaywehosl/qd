package cip

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	connectip "github.com/quic-go/connect-ip-go"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/yosida95/uritemplate/v3"

	"github.com/jaywehosl/qd/internal/qsrv"
	"github.com/jaywehosl/qd/internal/qsrv/uplink/quicconn"
	"github.com/jaywehosl/qd/internal/update"
)

type Client struct {
	qc *quicconn.Conn
	h3 *http3.Transport
	cc *http3.ClientConn
	ip *connectip.Conn

	auth   string
	device string
	token  string
}

func (c *Client) H3Conn() *http3.ClientConn { return c.cc }

func (c *Client) Hidden() bool { return c.qc.QUIC().ConnectionState().TLS.ECHAccepted }

func (c *Client) WritePacket(b []byte) (icmp []byte, err error) { return c.ip.WritePacket(b) }

func (c *Client) ReadPacket(b []byte) (int, error) { return c.ip.ReadPacket(b) }

func (c *Client) LocalPrefixes(ctx context.Context) ([]netip.Prefix, error) {
	return c.ip.LocalPrefixes(ctx)
}

func (c *Client) Reaches(ctx context.Context) error { return c.qc.Reaches(ctx) }

func (c *Client) Migrate(ctx context.Context, laddr *net.UDPAddr) error {
	return c.qc.Migrate(ctx, laddr)
}

func (c *Client) Close() error {
	err := c.ip.Close()
	c.h3.Close()
	c.qc.Close()
	return err
}

func DialAuth(ctx context.Context, endpoint string, tmpl *uritemplate.Template, tlsConf *tls.Config, token, device, route, authURL string, keep func(fd uintptr)) (*Client, error) {
	client, err := dialAuth(ctx, endpoint, tmpl, tlsConf, token, device, route, authURL, keep)
	if !Rejected0RTT(err) || tlsConf == nil || tlsConf.ClientSessionCache == nil {
		return client, err
	}
	tlsConf.ClientSessionCache.Put(tlsConf.ServerName, nil)
	return dialAuth(ctx, endpoint, tmpl, tlsConf, token, device, route, authURL, keep)
}

type carried struct {
	conn *connectip.Conn
	err  error
}

type opening struct {
	h3 *http3.Transport
	cc *http3.ClientConn
	ip chan carried
}

func (o *opening) close() {
	o.h3.Close()
	if o.ip == nil {
		return
	}
	go func() {
		if got := <-o.ip; got.conn != nil {
			got.conn.Close()
		}
	}()
}

func dialAuth(ctx context.Context, endpoint string, tmpl *uritemplate.Template, tlsConf *tls.Config, token, device, route, authURL string, keep func(fd uintptr)) (*Client, error) {
	head := http.Header{}
	sign(&http.Request{Header: head}, token, device, route)
	carry := func(o *opening) {
		o.ip = make(chan carried, 1)
		go func() {
			conn, _, err := connectip.Dial(ctx, o.cc, tmpl, head)
			o.ip <- carried{conn, err}
		}()
	}

	var mu sync.Mutex
	opened := map[*quicconn.Conn]*opening{}
	settled := false
	qc, err := quicconn.Dialer{TLS: tlsConf, Keep: keep, Prove: func(round context.Context, qc *quicconn.Conn, first bool) error {
		h3tr := &http3.Transport{EnableDatagrams: true}
		o := &opening{h3: h3tr, cc: h3tr.NewClientConn(qc.QUIC())}
		if first {
			carry(o)
		}
		mu.Lock()
		late := settled
		if !late {
			opened[qc] = o
		}
		mu.Unlock()
		if late {
			o.close()
			return context.Canceled
		}
		_, err := greet(round, o.cc, http3.MethodGet0RTT, token, device, route, authURL)
		return err
	}}.Dial(ctx, endpoint)

	mu.Lock()
	settled = true
	won := opened[qc]
	delete(opened, qc)
	mu.Unlock()
	for _, o := range opened {
		o.close()
	}
	if err != nil {
		return nil, err
	}

	if won.ip == nil {
		carry(won)
	}
	var got carried
	select {
	case got = <-won.ip:
	case <-ctx.Done():
		got = carried{err: ctx.Err()}
	}
	if got.err != nil {
		won.close()
		qc.Close()
		return nil, got.err
	}
	return &Client{qc: qc, h3: won.h3, cc: won.cc, ip: got.conn, auth: authURL, token: token, device: device}, nil
}

func Rejected0RTT(err error) bool { return errors.Is(err, quic.Err0RTTRejected) }

func DialAuthConn(ctx context.Context, qc *quicconn.Conn, tmpl *uritemplate.Template, token, device, route, authURL string) (*Client, error) {
	h3tr := &http3.Transport{EnableDatagrams: true}
	cc := h3tr.NewClientConn(qc.QUIC())

	began := time.Now()
	var greetTook time.Duration
	greeted := make(chan error, 1)
	go func() {
		_, err := greet(ctx, cc, http3.MethodGet0RTT, token, device, route, authURL)
		greetTook = time.Since(began)
		greeted <- err
	}()

	head := http.Header{}
	sign(&http.Request{Header: head}, token, device, route)
	ipConn, _, ipErr := connectip.Dial(ctx, cc, tmpl, head)
	ipTook := time.Since(began)
	err := <-greeted
	if max(greetTook, ipTook) > slowStart {
		fmt.Printf("cip      slow start: greet %d ms (%v), connect-ip %d ms (%v)\n", greetTook.Milliseconds(), err, ipTook.Milliseconds(), ipErr)
	}
	if err != nil || ipErr != nil {
		if ipConn != nil {
			ipConn.Close()
		}
		h3tr.Close()
		qc.Close()
		if err != nil {
			return nil, err
		}
		return nil, ipErr
	}
	return &Client{qc: qc, h3: h3tr, cc: cc, ip: ipConn, auth: authURL, token: token, device: device}, nil
}

const slowStart = 2 * time.Second

type roundTripper interface {
	RoundTrip(*http.Request) (*http.Response, error)
}

func sign(req *http.Request, token, device, route string) {
	req.Header.Set(qsrv.HeaderToken, token)
	if device != "" {
		req.Header.Set(qsrv.HeaderDevice, device)
	}
	if route == "" {
		route = qsrv.HereExit
	}
	req.Header.Set(qsrv.HeaderRoute, route)
	update.Stamp(req.Header)
}

func greet(ctx context.Context, rt roundTripper, method, token, device, route, url string) (http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	sign(req, token, device, route)

	rsp, err := rt.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("the node did not answer: %w", err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusNoContent {
		return nil, fmt.Errorf("the node refused this subscription")
	}
	return rsp.Header, nil
}

func (c *Client) Steer(ctx context.Context, route string) error {
	_, err := greet(ctx, c.cc, http.MethodGet, c.token, c.device, route, c.auth)
	return err
}

func (c *Client) WritePacketMarked(b []byte, mark uint64) (icmp []byte, err error) {
	return c.ip.WritePacketMarked(b, mark)
}

func (c *Client) ReadPacketMarked(b []byte) (int, uint64, error) {
	return c.ip.ReadPacketMarked(b)
}

func (c *Client) Received() uint64 {
	qc := c.qc.QUIC()
	if qc == nil {
		return 0
	}
	return qc.ConnectionStats().PacketsReceived
}

func (c *Client) Alive() bool {
	qc := c.qc.QUIC()
	return qc != nil && qc.Context().Err() == nil
}

func (c *Client) Ask(ctx context.Context, route string) error {
	if !c.Alive() {
		return fmt.Errorf("the session is closed")
	}
	return c.Steer(ctx, route)
}

func (c *Client) DatagramLimit() int {
	_ = c.qc.SendDatagram(make([]byte, 1<<16))
	return c.qc.MaxDatagramSize()
}
