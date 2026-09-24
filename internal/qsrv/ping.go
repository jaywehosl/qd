package qsrv

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"

	"github.com/jaywehosl/quic-diver/internal/qsrv/server/netstack"
)

func (d steered) Ping(ctx context.Context, dst netip.Addr, ttl uint8, payload []byte) (netstack.Echo, error) {
	p, ok := d.node.dialerFor(ctx, d.grant, d.s.heading(), d.hops).(netstack.Pinger)
	if !ok {
		return netstack.Echo{}, errors.New("this route cannot ping")
	}
	return p.Ping(ctx, dst, ttl, payload)
}

func (d refusing) Ping(context.Context, netip.Addr, uint8, []byte) (netstack.Echo, error) {
	return netstack.Echo{}, d.why
}

func (c chained) Ping(ctx context.Context, dst netip.Addr, ttl uint8, payload []byte) (netstack.Echo, error) {
	host := netip.AddrPortFrom(dst, 0).String()
	head := http.Header{}
	head.Set(HeaderProto, "icmp")
	head.Set(HeaderHops, strconv.Itoa(c.hops))
	head.Set(HeaderTTL, strconv.Itoa(int(ttl)))
	head.Set(HeaderEcho, base64.StdEncoding.EncodeToString(payload))

	req := (&http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Scheme: "https", Host: host},
		Host:   host,
		Header: head,
	}).WithContext(ctx)
	rsp, err := c.cc.RoundTrip(req)
	if err != nil {
		return netstack.Echo{}, err
	}
	rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return netstack.Echo{}, fmt.Errorf("%s: %s", c.endpoint, rsp.Status)
	}

	from, err := netip.ParseAddr(rsp.Header.Get(HeaderFrom))
	if err != nil {
		return netstack.Echo{}, err
	}
	var typ, code, ttlBack uint8
	if _, err := fmt.Sscanf(rsp.Header.Get(HeaderICMP), "%d/%d/%d", &typ, &code, &ttlBack); err != nil {
		return netstack.Echo{}, err
	}
	return netstack.Echo{From: from, Type: typ, Code: code, TTL: ttlBack}, nil
}

func (n *Node) servePing(w http.ResponseWriter, r *http.Request, dialer netstack.Dialer, dst netip.Addr) {
	p, ok := dialer.(netstack.Pinger)
	payload, err := base64.StdEncoding.DecodeString(r.Header.Get(HeaderEcho))
	if !ok || err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	ttl, _ := strconv.Atoi(r.Header.Get(HeaderTTL))

	ctx, cancel := context.WithTimeout(r.Context(), netstack.EchoWait)
	defer cancel()
	got, err := p.Ping(ctx, dst, uint8(ttl), payload)
	if err != nil {
		w.WriteHeader(http.StatusGatewayTimeout)
		return
	}
	w.Header().Set(HeaderFrom, got.From.String())
	w.Header().Set(HeaderICMP, fmt.Sprintf("%d/%d/%d", got.Type, got.Code, got.TTL))
	w.WriteHeader(http.StatusOK)
}
