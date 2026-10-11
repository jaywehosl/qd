package clientdns

import (
	"errors"
	"net"
	"net/netip"
	"slices"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/qd/internal/dnsproxy"
	"github.com/jaywehosl/qd/internal/ippkt"
)

type Ask func(endpoint, op, auth string, body, out any) error

type Config struct {
	Node     string
	Token    string
	Ask      Ask
	Adblock  func() bool
	Seen     func(name string)
	Answered func(name string, addrs []netip.Addr)
	Native   func(name string) bool
	Say      func(format string, args ...any)
	Device   string
	Exit     func() bool
}

type Stats struct {
	Queries, Hits, Upstream, Failed, Blocked, NoV6 uint64
}

type Resolver struct {
	conn    *net.UDPConn
	node    atomic.Pointer[string]
	token   string
	device  string
	exit    func() bool
	ask     Ask
	say     func(string, ...any)
	adblock func() bool
	seen    func(name string)
	learned func(name string, addrs []netip.Addr)
	native  func(name string) bool

	cache *dnsproxy.Resolver

	queries, hits, upstream, failed, refused, noV6 atomic.Uint64
	lastOK                                         atomic.Int64
	six                                            atomic.Bool
}

func New(cfg Config) (*Resolver, error) {
	if cfg.Ask == nil {
		return nil, errors.New("clientdns: no way to ask the node")
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, err
	}

	r := &Resolver{conn: conn, token: cfg.Token, device: cfg.Device, exit: cfg.Exit, ask: cfg.Ask, say: cfg.Say, adblock: cfg.Adblock, seen: cfg.Seen, learned: cfg.Answered, native: cfg.Native}
	r.node.Store(&cfg.Node)
	r.cache = dnsproxy.New(dnsproxy.Config{Cache: cacheSize, MaxTTL: cacheMaxTTL, Stale: cacheStale, Negative: cacheMaxTTL, Forward: r.fromNode})
	return r, nil
}

const (
	cacheSize   = 2048
	cacheMaxTTL = 5 * time.Minute
	cacheStale  = 30 * time.Second
)

func (r *Resolver) Addr() string { return r.conn.LocalAddr().String() }

func (r *Resolver) SetNode(node string) {
	if node == "" {
		return
	}
	r.node.Store(&node)
}

func (r *Resolver) Serve(stop <-chan struct{}) {
	buf := make([]byte, 4096)

	for {
		select {
		case <-stop:
			return
		default:
		}

		r.conn.SetReadDeadline(time.Now().Add(readStep))
		n, from, err := r.conn.ReadFromUDP(buf)
		if err != nil || n < 12 {
			continue
		}

		query := make([]byte, n)
		copy(query, buf[:n])
		go r.handle(query, from)
	}
}

const readStep = 500 * time.Millisecond

func (r *Resolver) Close() {
	if r != nil && r.conn != nil {
		r.conn.Close()
	}
}

func (r *Resolver) Interrupt() {
	if r != nil && r.conn != nil {
		r.conn.SetReadDeadline(time.Now())
	}
}

func (r *Resolver) handle(query []byte, from *net.UDPAddr) {
	r.queries.Add(1)

	name, qtype, ok := dnsproxy.Question(query)
	if !ok {
		r.failed.Add(1)
		return
	}

	if qtype == 28 && !r.six.Load() {
		r.noV6.Add(1)
		r.conn.WriteToUDP(dnsproxy.NoData(query), from)
		return
	}

	began := time.Now()
	answer, hit, err := r.cache.Answer(query)
	r.tell("dns: %s %dms hit=%v err=%v", name, time.Since(began).Milliseconds(), hit, err)

	if err != nil {
		r.failed.Add(1)
		r.conn.WriteToUDP(dnsproxy.ServFail(query), from)
		return
	}

	r.lastOK.Store(time.Now().UnixNano())
	if hit {
		r.hits.Add(1)
	} else {
		r.upstream.Add(1)
	}
	if r.seen != nil && len(answer) > 3 && answer[3]&0x0F == 0 {
		r.seen(name)
	}
	addrs := dnsproxy.Addrs(answer)
	if r.native != nil && r.native(name) && slices.ContainsFunc(addrs, ippkt.StandIn.Contains) {
		answer, addrs = dnsproxy.NoData(query), nil
	}
	if r.learned != nil {
		r.learned(name, addrs)
	}
	r.conn.WriteToUDP(answer, from)
}

func (r *Resolver) fromNode(query []byte) ([]byte, error) {
	var answer struct {
		Answer  []byte `json:"answer"`
		Blocked bool   `json:"blocked"`
	}
	body := map[string]any{
		"query": query, "device": r.device,
		"exit":    r.exit != nil && r.exit(),
		"adblock": r.adblock != nil && r.adblock(),
	}
	if err := r.ask(r.asking(), "dns", r.token, body, &answer); err != nil {
		return nil, err
	}
	if answer.Blocked {
		r.refused.Add(1)
	}
	if len(answer.Answer) < 12 {
		return nil, errors.New("the node returned nothing")
	}

	copy(answer.Answer[0:2], query[0:2])
	return answer.Answer, nil
}

func (r *Resolver) Flush() { r.cache.Flush() }

func (r *Resolver) KeepWarm(stop <-chan struct{}) {
	tick := time.NewTicker(warmStep)
	defer tick.Stop()

	for {
		if time.Since(time.Unix(0, r.lastOK.Load())) >= warmStep {
			if err := r.ask(r.asking(), "whoami", r.token, nil, nil); err != nil {
				r.tell("dns: the node went quiet between queries: %v", err)
			}
		}
		select {
		case <-stop:
			return
		case <-tick.C:
		}
	}
}

const warmStep = 20 * time.Second

func (r *Resolver) RTT() int {
	began := time.Now()
	if err := r.ask(r.asking(), "whoami", r.token, nil, nil); err != nil {
		return -1
	}
	return int(time.Since(began).Milliseconds())
}

func (r *Resolver) Stats() Stats {
	return Stats{
		Queries:  r.queries.Load(),
		Hits:     r.hits.Load(),
		Upstream: r.upstream.Load(),
		Failed:   r.failed.Load(),
		Blocked:  r.refused.Load(),
		NoV6:     r.noV6.Load(),
	}
}

func (r *Resolver) asking() string {
	if held := r.node.Load(); held != nil {
		return *held
	}
	return ""
}

func (r *Resolver) tell(format string, args ...any) {
	if r.say != nil {
		r.say(format, args...)
	}
}

func (r *Resolver) CarryV6(on bool) { r.six.Store(on) }
