package qsrv

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go/http3"

	"github.com/jaywehosl/quic-diver/internal/qsrv/server/netstack"
)

const (
	steerKeep = 12 * time.Hour
	steerCap  = 1 << 18
	steerRest = 20 * time.Second
)

type steerMark struct {
	until int64
	name  string
}

type steerTable struct {
	mu       sync.RWMutex
	addrs    map[netip.Addr]steerMark
	prefixes []netip.Prefix
	resting  atomic.Int64
}

func (t *steerTable) any() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.addrs) > 0 || len(t.prefixes) > 0
}

func (t *steerTable) has(a netip.Addr) bool {
	a = a.Unmap()
	t.mu.RLock()
	defer t.mu.RUnlock()
	if held, ok := t.addrs[a]; ok && held.until > time.Now().Unix() {
		return true
	}
	for _, p := range t.prefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func (n *Node) SteerPrefixes(prefixes []netip.Prefix) {
	n.steer.mu.Lock()
	n.steer.prefixes = prefixes
	n.steer.mu.Unlock()
}

func (n *Node) Steer(name string, addrs []netip.Addr) {
	if len(addrs) == 0 {
		return
	}
	now := time.Now().Unix()
	until := now + int64(steerKeep/time.Second)

	n.steer.mu.Lock()
	defer n.steer.mu.Unlock()
	if n.steer.addrs == nil {
		n.steer.addrs = map[netip.Addr]steerMark{}
	}
	for _, a := range addrs {
		n.steer.addrs[a.Unmap()] = steerMark{until: until, name: name}
	}
	if len(n.steer.addrs) <= steerCap {
		return
	}
	for a, held := range n.steer.addrs {
		if held.until <= now {
			delete(n.steer.addrs, a)
		}
	}
	for a := range n.steer.addrs {
		if len(n.steer.addrs) <= steerCap/2 {
			break
		}
		delete(n.steer.addrs, a)
	}
}

func (n *Node) SteerKeepOnly(still func(name string) bool) int {
	n.steer.mu.Lock()
	defer n.steer.mu.Unlock()
	gone := 0
	for a, held := range n.steer.addrs {
		if !still(held.name) {
			delete(n.steer.addrs, a)
			gone++
		}
	}
	return gone
}

func (n *Node) steerExit(ctx context.Context, seat, session uint32) (*http3.ClientConn, string, error) {
	if time.Now().UnixNano() < n.steer.resting.Load() {
		return nil, "", fmt.Errorf("the exit nodes did not answer a moment ago")
	}
	cc, endpoint, err := n.raceExit(ctx, AnyExit, seat, session)
	if err != nil {
		n.steer.resting.Store(time.Now().Add(steerRest).UnixNano())
	}
	return cc, endpoint, err
}

type steering struct {
	node  *Node
	grant Grant
	hops  int
	local netstack.NetDialer
}

func (d steering) via(ctx context.Context, dst netip.Addr) netstack.Dialer {
	if !d.node.steer.has(dst) {
		return d.local
	}
	won, endpoint, err := d.node.steerExit(ctx, d.grant.Seat, d.grant.Session)
	if err != nil {
		return d.local
	}
	d.node.transits.Add(1)
	return chained{cc: won, ls: d.node.links, endpoint: endpoint, seat: d.grant.Seat, hops: d.hops - 1}
}

func (d steering) DialTCP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	return d.via(ctx, dst.Addr()).DialTCP(ctx, dst)
}

func (d steering) DialUDP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	return d.via(ctx, dst.Addr()).DialUDP(ctx, dst)
}

func (d steering) Ping(ctx context.Context, dst netip.Addr, ttl uint8, payload []byte) (netstack.Echo, error) {
	return d.local.Ping(ctx, dst, ttl, payload)
}

func (n *Node) AskExit(ctx context.Context, op string, body []byte) ([]byte, error) {
	cc, endpoint, err := n.steerExit(ctx, 0, 0)
	if err != nil {
		return nil, err
	}
	at := where{endpoint, 0}
	n.links.hold(at)
	defer n.links.release(at)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://"+endpoint+RPCPath+op, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	rsp, err := cc.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s: %s", endpoint, op, rsp.Status)
	}
	return io.ReadAll(io.LimitReader(rsp.Body, maxAsk))
}
