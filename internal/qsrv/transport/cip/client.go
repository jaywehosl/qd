package cip

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/netip"

	connectip "github.com/quic-go/connect-ip-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/yosida95/uritemplate/v3"

	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink/quicconn"
)

type Client struct {
	qc *quicconn.Conn
	h3 *http3.Transport
	cc *http3.ClientConn
	ip *connectip.Conn

	auth  string
	token string
}

func (c *Client) H3Conn() *http3.ClientConn { return c.cc }

func Dial(ctx context.Context, endpoint string, tmpl *uritemplate.Template, tlsConf *tls.Config) (*Client, *http.Response, error) {
	qcAny, err := quicconn.Dialer{TLS: ensureH3(tlsConf)}.Dial(ctx, endpoint)
	if err != nil {
		return nil, nil, err
	}
	qc := qcAny.(*quicconn.Conn)

	h3tr := &http3.Transport{EnableDatagrams: true}
	cc := h3tr.NewClientConn(qc.QUIC())

	ipConn, rsp, err := connectip.Dial(ctx, cc, tmpl)
	if err != nil {
		h3tr.Close()
		qc.Close()
		return nil, nil, err
	}
	return &Client{qc: qc, h3: h3tr, cc: cc, ip: ipConn}, rsp, nil
}

func (c *Client) WritePacket(b []byte) (icmp []byte, err error) { return c.ip.WritePacket(b) }

func (c *Client) ReadPacket(b []byte) (int, error) { return c.ip.ReadPacket(b) }

func (c *Client) LocalPrefixes(ctx context.Context) ([]netip.Prefix, error) {
	return c.ip.LocalPrefixes(ctx)
}

func (c *Client) Migrate(ctx context.Context, laddr *net.UDPAddr) error {
	return c.qc.Migrate(ctx, laddr)
}

func (c *Client) Close() error {
	err := c.ip.Close()
	c.h3.Close()
	c.qc.Close()
	return err
}

func ensureH3(t *tls.Config) *tls.Config {
	if t == nil {
		t = &tls.Config{}
	} else {
		t = t.Clone()
	}
	if len(t.NextProtos) == 0 {
		t.NextProtos = []string{http3.NextProtoH3}
	}
	return t
}

func DialAuth(ctx context.Context, endpoint string, tmpl *uritemplate.Template, tlsConf *tls.Config, token, route, authURL string, keep func(fd uintptr)) (*Client, *http.Response, error) {
	qcAny, err := quicconn.Dialer{TLS: ensureH3(tlsConf), Keep: keep}.Dial(ctx, endpoint)
	if err != nil {
		return nil, nil, err
	}
	qc := qcAny.(*quicconn.Conn)

	h3tr := &http3.Transport{EnableDatagrams: true}
	cc := h3tr.NewClientConn(qc.QUIC())

	if authURL != "" {
		if err := greet(ctx, cc, token, route, authURL); err != nil {
			h3tr.Close()
			qc.Close()
			return nil, nil, err
		}
	}

	ipConn, rsp, err := connectip.Dial(ctx, cc, tmpl)
	if err != nil {
		h3tr.Close()
		qc.Close()
		return nil, nil, err
	}
	return &Client{qc: qc, h3: h3tr, cc: cc, ip: ipConn, auth: authURL, token: token}, rsp, nil
}

func greet(ctx context.Context, cc *http3.ClientConn, token, route, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set(tokenHeader, token)
	if route == "" {
		route = hereExit
	}
	req.Header.Set(routeHeader, route)

	rsp, err := cc.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("the node did not answer: %w", err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("the node refused this subscription")
	}
	return nil
}

const (
	tokenHeader = "Qd-Token"
	routeHeader = "Qd-Route"
	hereExit    = "here"
)

func (c *Client) Steer(ctx context.Context, route string) error {
	if c.auth == "" {
		return nil
	}
	return greet(ctx, c.cc, c.token, route, c.auth)
}

func (c *Client) WritePacketMarked(b []byte, mark uint64) (icmp []byte, err error) {
	return c.ip.WritePacketMarked(b, mark)
}

func (c *Client) ReadPacketMarked(b []byte) (int, uint64, error) {
	return c.ip.ReadPacketMarked(b)
}

func (c *Client) Alive() bool {
	qc := c.qc.QUIC()
	if qc == nil {
		return false
	}
	return qc.Context().Err() == nil
}

func (c *Client) Ask(ctx context.Context, route string) error {
	if !c.Alive() {
		return fmt.Errorf("the session is closed")
	}
	if c.auth == "" {
		return fmt.Errorf("nothing to ask: this client never greeted the node")
	}
	return greet(ctx, c.cc, c.token, route, c.auth)
}

func (c *Client) DatagramLimit() int {
	_ = c.qc.SendDatagram(make([]byte, 1<<16))
	return c.qc.MaxDatagramSize()
}
