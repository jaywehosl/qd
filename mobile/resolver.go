package qdmobile

import (
	"encoding/json"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/quic-diver/internal/clientapi"
	"github.com/jaywehosl/quic-diver/internal/dnsproxy"
)

type dnsStats struct {
	queries  atomic.Uint64
	hits     atomic.Uint64
	upstream atomic.Uint64
	failed   atomic.Uint64
	blocked  atomic.Uint64
	noV6     atomic.Uint64
}

type track struct {
	Name  string `json:"name"`
	Ms    int64  `json:"nodeMs"`
	Whole int64  `json:"wholeMs"`
	Kind  string `json:"kind,omitempty"`
	Hit   bool   `json:"hit"`
	Err   string `json:"err,omitempty"`
	At    int64  `json:"sinceUpMs"`
}

type resolver struct {
	conn  *net.UDPConn
	node  atomic.Pointer[string]
	token string
	ask   func(endpoint, op, auth string, body any, out any) error
	seen  *clientapi.Visits

	born   time.Time
	mu     sync.Mutex
	recent []track

	stats dnsStats
}

const recentKept = 64

func newResolver(node, token string, seen *clientapi.Visits,
	ask func(endpoint, op, auth string, body any, out any) error) (*resolver, error) {

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	r := &resolver{conn: conn, token: token, ask: ask, seen: seen, born: time.Now()}
	r.node.Store(&node)
	return r, nil
}

func (r *resolver) Addr() string { return r.conn.LocalAddr().String() }

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
		if err != nil || n < 12 {
			continue
		}

		query := make([]byte, n)
		copy(query, buf[:n])
		go r.handle(query, from)
	}
}

func (r *resolver) Close() {
	if r != nil && r.conn != nil {
		r.conn.Close()
	}
}

func (r *resolver) handle(query []byte, from *net.UDPAddr) {
	entered := time.Now()
	r.stats.queries.Add(1)

	name, qtype, ok := dnsproxy.Question(query)
	if !ok {
		r.stats.failed.Add(1)
		return
	}

	if r.blocks(name) {
		r.stats.blocked.Add(1)
		r.conn.WriteToUDP(dnsproxy.Refused(query), from)
		r.note(track{Name: name, Kind: "blocked", Whole: since(entered), At: since(r.born)})
		return
	}

	if qtype == 28 {
		r.stats.noV6.Add(1)
		r.conn.WriteToUDP(dnsproxy.NoData(query), from)
		return
	}

	began := time.Now()
	answer, hit, err := r.fetch(query)
	spent := since(began)
	say("dns: %s %dms hit=%v err=%v", name, spent, hit, err)

	if err != nil {
		r.stats.failed.Add(1)
		r.conn.WriteToUDP(dnsproxy.ServFail(query), from)
		r.note(track{Name: name, Ms: spent, Whole: since(entered), Err: err.Error(), At: since(r.born)})
		return
	}

	if hit {
		r.stats.hits.Add(1)
	} else {
		r.stats.upstream.Add(1)
	}
	r.conn.WriteToUDP(answer, from)
	r.note(track{Name: name, Ms: spent, Whole: since(entered), Hit: hit, At: since(r.born)})
}

func (r *resolver) fetch(query []byte) ([]byte, bool, error) {
	var answer struct {
		Answer []byte `json:"answer"`
		Hit    bool   `json:"hit"`
	}
	if err := r.ask(r.asking(), "dns", r.token, map[string]any{"query": query}, &answer); err != nil {
		return nil, false, err
	}
	if len(answer.Answer) < 12 {
		return nil, false, errors.New("the node returned nothing")
	}

	copy(answer.Answer[0:2], query[0:2])
	return answer.Answer, answer.Hit, nil
}

func (r *resolver) blocks(name string) bool {
	if r.seen == nil {
		return false
	}
	return r.seen.Query(name)
}

func (r *resolver) note(t track) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.recent = append(r.recent, t)
	if len(r.recent) > recentKept {
		r.recent = append([]track{}, r.recent[len(r.recent)-recentKept:]...)
	}
}

func (r *resolver) seenSoFar() []track {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]track{}, r.recent...)
}

func (r *resolver) LinesJSON() string {
	blob, err := json.Marshal(r.seenSoFar())
	if err != nil {
		return "[]"
	}
	return string(blob)
}

func since(t time.Time) int64 { return time.Since(t).Milliseconds() }

func (r *resolver) rtt() int {
	began := time.Now()
	if err := r.ask(r.asking(), "whoami", r.token, nil, nil); err != nil {
		return -1
	}
	return int(since(began))
}

func (r *resolver) keepWarm(stop <-chan struct{}) {
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()

	for {
		if err := r.ask(r.asking(), "whoami", r.token, nil, nil); err != nil {
			say("dns: the node went quiet between queries: %v", err)
		}
		select {
		case <-stop:
			return
		case <-tick.C:
		}
	}
}
