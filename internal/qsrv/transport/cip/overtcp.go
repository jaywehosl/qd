package cip

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync/atomic"

	"golang.org/x/net/http2"

	"github.com/jaywehosl/quic-diver/internal/roads"
)

type Over struct {
	conn   net.Conn
	cc     *http2.ClientConn
	auth   string
	token  string
	device string
	given  atomic.Pointer[netip.Prefix]
}

func DialOver(ctx context.Context, endpoint string, tlsConf *tls.Config, token, device, route, authURL string) (*Over, error) {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, err
	}

	conf := tlsConf.Clone()
	conf.ServerName = host
	conf.NextProtos = []string{"h2"}

	raw, err := roads.ReachTCP(ctx, nil, endpoint)
	if err != nil {
		return nil, err
	}
	held := tls.Client(raw, conf)
	if err := held.HandshakeContext(ctx); err != nil {
		raw.Close()
		return nil, err
	}
	if state := held.ConnectionState(); state.NegotiatedProtocol != "h2" {
		held.Close()
		return nil, fmt.Errorf("the node offered %q, not h2", state.NegotiatedProtocol)
	}

	tr := &http2.Transport{}
	cc, err := tr.NewClientConn(held)
	if err != nil {
		held.Close()
		return nil, err
	}

	over := &Over{conn: held, cc: cc, auth: authURL, token: token, device: device}
	if authURL != "" {
		if err := over.greet(ctx, route); err != nil {
			over.Close()
			return nil, err
		}
	}
	return over, nil
}

func (o *Over) greet(ctx context.Context, route string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.auth, nil)
	if err != nil {
		return err
	}
	req.Header.Set(tokenHeader, o.token)
	if o.device != "" {
		req.Header.Set(deviceHeader, o.device)
	}
	if route == "" {
		route = hereExit
	}
	req.Header.Set(routeHeader, route)

	rsp, err := o.cc.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("the node did not answer: %w", err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("the node refused this subscription")
	}
	if given := rsp.Header.Get(addrHeader); given != "" {
		if held, err := netip.ParsePrefix(given); err == nil {
			o.given.Store(&held)
		}
	}
	return nil
}

func (o *Over) H2Conn() *http2.ClientConn { return o.cc }

func (o *Over) Steer(ctx context.Context, route string) error {
	if o.auth == "" {
		return nil
	}
	return o.greet(ctx, route)
}

func (o *Over) Ask(ctx context.Context, route string) error {
	if !o.Alive() {
		return fmt.Errorf("the session is closed")
	}
	return o.greet(ctx, route)
}

func (o *Over) Alive() bool { return o.cc != nil && o.cc.CanTakeNewRequest() }

func (o *Over) Close() error { return o.conn.Close() }

func (o *Over) LocalPrefixes(context.Context) ([]netip.Prefix, error) {
	if held := o.given.Load(); held != nil {
		return []netip.Prefix{*held}, nil
	}
	return nil, fmt.Errorf("the node named no address for this path")
}

func (o *Over) ReadPacket(b []byte) (int, error) {
	return 0, fmt.Errorf("no datagrams over tcp")
}

func (o *Over) WritePacket(b []byte) ([]byte, error) {
	return nil, fmt.Errorf("no datagrams over tcp")
}

func (o *Over) DatagramLimit() int { return 0 }

func (o *Over) Migrate(ctx context.Context, laddr *net.UDPAddr) error {
	return fmt.Errorf("a tcp path does not migrate")
}
