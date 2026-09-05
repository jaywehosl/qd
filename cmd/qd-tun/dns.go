//go:build windows

package main

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/quic-diver/internal/dnsproxy"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
)

type dnsStats struct {
	queries  atomic.Uint64
	hits     atomic.Uint64
	upstream atomic.Uint64
	failed   atomic.Uint64
	blocked  atomic.Uint64
	noV6     atomic.Uint64
}

type resolver struct {
	said atomic.Uint64

	conn  *net.UDPConn
	node  atomic.Pointer[string]
	token string

	mu   sync.Mutex
	seen func(name string) (block bool)

	stats dnsStats
}

func newResolver(listen string, key qdcrypt.Key, node, token string) (*resolver, error) {
	addr, err := net.ResolveUDPAddr("udp", listen)
	if err != nil {
		return nil, err
	}

	var conn *net.UDPConn
	deadline := time.Now().Add(10 * time.Second)
	for {
		conn, err = net.ListenUDP("udp", addr)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("dns listen %s: %w", listen, err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	r := &resolver{conn: conn, token: token}
	r.node.Store(&node)
	return r, nil
}

func (r *resolver) Addr() string { return r.conn.LocalAddr().String() }

func (r *resolver) keepWarm(stop <-chan struct{}) {
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()

	for {
		if err := nodeTalk.Ask(r.asking(), "whoami", r.token, nil, nil); err != nil {
			fmt.Printf("dns      the node went quiet between queries: %v\n", err)
		}
		select {
		case <-stop:
			return
		case <-tick.C:
		}
	}
}

// Rebind — след эпохи UDP: там резолвер держал свой сокет к узлу и его надо
// было пересоздавать при смене сети. Теперь запрос уезжает общим QUIC-каналом,
// и пересоздавать нечего.
func (r *resolver) Rebind() {}

func (r *resolver) Close() { r.conn.Close() }

func (r *resolver) Interrupt() {
	if r != nil && r.conn != nil {
		r.conn.SetReadDeadline(time.Now())
	}
}

func (r *resolver) OnQuery(fn func(name string) bool) {
	r.mu.Lock()
	r.seen = fn
	r.mu.Unlock()
}

func (r *resolver) askSeen(name string) bool {
	r.mu.Lock()
	fn := r.seen
	r.mu.Unlock()
	if fn == nil {
		return false
	}
	return fn(name)
}

func (r *resolver) Serve(stop <-chan struct{}) {
	buf := make([]byte, 4096)

	for {
		select {
		case <-stop:
			return
		default:
		}

		r.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, from, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		if n < 12 {
			continue
		}

		query := make([]byte, n)
		copy(query, buf[:n])
		go r.handle(query, from)
	}
}

func (r *resolver) handle(query []byte, from *net.UDPAddr) {
	r.stats.queries.Add(1)

	name, qtype, ok := dnsproxy.Question(query)
	if !ok {
		r.stats.failed.Add(1)
		return
	}

	if r.askSeen(name) {
		r.stats.blocked.Add(1)
		r.conn.WriteToUDP(dnsproxy.Refused(query), from)
		return
	}

	if qtype == 28 {
		r.stats.noV6.Add(1)
		r.conn.WriteToUDP(dnsproxy.NoData(query), from)
		return
	}

	answer, hit, err := r.ask(query)
	if err != nil {
		r.stats.failed.Add(1)
		r.conn.WriteToUDP(dnsproxy.ServFail(query), from)
		return
	}
	if hit {
		r.stats.hits.Add(1)
	} else {
		r.stats.upstream.Add(1)
	}
	r.conn.WriteToUDP(answer, from)
}

func (r *resolver) ask(query []byte) ([]byte, bool, error) {
	var answer struct {
		Answer []byte `json:"answer"`
		Hit    bool   `json:"hit"`
	}
	if err := nodeTalk.Ask(r.asking(), "dns", r.token, map[string]any{"query": query}, &answer); err != nil {
		return nil, false, err
	}
	if len(answer.Answer) < 12 {
		return nil, false, errors.New("dns: the node returned nothing")
	}

	copy(answer.Answer[0:2], query[0:2])
	return answer.Answer, answer.Hit, nil
}

func (r *resolver) Line() string {
	q := r.stats.queries.Load()
	h := r.stats.hits.Load()

	rate := 0.0
	if q > 0 {
		rate = 100 * float64(h) / float64(q)
	}

	return fmt.Sprintf("dns      %d queries, %.0f%% answered from the node's cache | resolved %d, blocked %d, no ipv6 %d, failed %d",
		q, rate, r.stats.upstream.Load(), r.stats.blocked.Load(),
		r.stats.noV6.Load(), r.stats.failed.Load())
}

func (r *resolver) asking() string {
	if held := r.node.Load(); held != nil {
		return *held
	}
	return ""
}

func (r *resolver) SetNode(node string) {
	if node == "" {
		return
	}
	r.node.Store(&node)
}
