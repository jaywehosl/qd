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

	"github.com/jaywehosl/quic-diver/internal/qsrv"
	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink/quicconn"
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

func DialAuth(ctx context.Context, endpoint string, tmpl *uritemplate.Template, tlsConf *tls.Config, token, device, route, authURL string, keep func(fd uintptr)) (*Client, error) {
	qc, err := quicconn.Dialer{TLS: tlsConf, Keep: keep}.Dial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	return DialAuthConn(ctx, qc, tmpl, token, device, route, authURL)
}

func DialAuthConn(ctx context.Context, qc *quicconn.Conn, tmpl *uritemplate.Template, token, device, route, authURL string) (*Client, error) {
	h3tr := &http3.Transport{EnableDatagrams: true}
	cc := h3tr.NewClientConn(qc.QUIC())

	if _, err := greet(ctx, cc, token, device, route, authURL); err != nil {
		h3tr.Close()
		qc.Close()
		return nil, err
	}

	ipConn, _, err := connectip.Dial(ctx, cc, tmpl)
	if err != nil {
		h3tr.Close()
		qc.Close()
		return nil, err
	}
	return &Client{qc: qc, h3: h3tr, cc: cc, ip: ipConn, auth: authURL, token: token, device: device}, nil
}

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
}

func greet(ctx context.Context, rt roundTripper, token, device, route, url string) (http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	_, err := greet(ctx, c.cc, c.token, c.device, route, c.auth)
	return err
}

func (c *Client) WritePacketMarked(b []byte, mark uint64) (icmp []byte, err error) {
	return c.ip.WritePacketMarked(b, mark)
}

func (c *Client) ReadPacketMarked(b []byte) (int, uint64, error) {
	return c.ip.ReadPacketMarked(b)
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
