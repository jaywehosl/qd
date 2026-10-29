package relay

import (
	"cmp"
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"net/netip"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/xtls/xray-core/transport/internet"

	"github.com/jaywehosl/qd/internal/clientapi"
	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/pace"
	"github.com/jaywehosl/qd/internal/qcli"
	"github.com/jaywehosl/qd/internal/qsrv/transport/cip"
	"github.com/jaywehosl/qd/internal/vlesslink"
)

type Pool struct {
	Keep   func(fd uintptr)
	Lookup func(host string) []netip.Addr
	Riding func() int
	Fresh  func(hosts []string)

	mu      sync.Mutex
	mode    string
	open    map[int]*member
	broken  map[int]string
	stats   map[int]*stat
	probing atomic.Bool
	kept    sync.Once
}

type stat struct {
	reach, shake atomic.Int64
	checked      atomic.Int64
	fault        atomic.Pointer[string]
}

type member struct {
	id     int
	link   string
	dialer *Dialer
	stat   *stat
}

func (p *Pool) riding() int {
	if p.Riding == nil {
		return 0
	}
	return p.Riding()
}

func (p *Pool) Set(list []clientstate.Relay, mode string) bool {
	if p.Keep != nil {
		p.kept.Do(func() {
			internet.RegisterDialerController(func(_, _ string, c syscall.RawConn) error { return c.Control(p.Keep) })
		})
	}

	riding := p.riding()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.open == nil {
		p.open, p.broken, p.stats = map[int]*member{}, map[int]string{}, map[int]*stat{}
	}

	redial := mode != p.mode
	p.mode = mode

	want := map[int]clientstate.Relay{}
	for _, r := range list {
		if r.Enabled {
			want[r.ID] = r
		}
	}
	for id, m := range p.open {
		if w, keep := want[id]; keep && w.Link == m.link {
			continue
		}
		m.dialer.Close()
		delete(p.open, id)
		redial = redial || id == riding
	}
	var fresh []string
	for id, r := range want {
		if _, held := p.open[id]; held {
			continue
		}
		d, err := p.dial(r.Link, netip.Addr{})
		if err != nil {
			p.broken[id] = err.Error()
			fmt.Printf("relay    #%d does not open: %v\n", id, err)
			continue
		}
		delete(p.broken, id)
		if p.stats[id] == nil {
			p.stats[id] = &stat{}
		}
		p.open[id] = &member{id: id, link: r.Link, dialer: d, stat: p.stats[id]}
		fresh = append(fresh, d.host())
		fmt.Printf("relay    #%d %s %s\n", id, cmp.Or(d.Link.Name, "relay"), d.Link.Summary())
	}
	for id := range p.stats {
		if _, kept := want[id]; !kept {
			delete(p.stats, id)
			delete(p.broken, id)
		}
	}
	if len(fresh) > 0 && p.Fresh != nil {
		p.Fresh(fresh)
	}
	return redial
}

func (p *Pool) dial(link string, was netip.Addr) (*Dialer, error) {
	parsed, err := vlesslink.Parse(link)
	if err != nil {
		return nil, err
	}
	at := p.resolve(parsed.Host)
	if !at.IsValid() {
		at = was
	}
	return New(parsed, at)
}

const lookupWait = 4 * time.Second

func (p *Pool) resolve(host string) netip.Addr {
	if at, err := netip.ParseAddr(host); err == nil {
		return at.Unmap()
	}
	var found []netip.Addr
	if p.Lookup != nil {
		found = p.Lookup(host)
	}
	if len(found) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), lookupWait)
		found, _ = net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		cancel()
	}
	for _, at := range found {
		if at = at.Unmap(); at.Is4() {
			return at
		}
	}
	if len(found) > 0 {
		return found[0].Unmap()
	}
	return netip.Addr{}
}

func (m *member) via(ctx context.Context, endpoint string) (net.Conn, error) {
	return m.dialer.Dial(ctx, endpoint)
}

func (p *Pool) members() []*member {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*member, 0, len(p.open))
	for _, m := range p.open {
		out = append(out, m)
	}
	slices.SortFunc(out, func(a, b *member) int { return a.id - b.id })
	return out
}

func (p *Pool) Now() ([]qcli.Relay, bool) {
	p.mu.Lock()
	mode := p.mode
	p.mu.Unlock()
	if mode == clientstate.RelayOff {
		return nil, false
	}
	var out []qcli.Relay
	for _, m := range p.members() {
		out = append(out, qcli.Relay{ID: m.id, Name: cmp.Or(m.dialer.Link.Name, m.dialer.Link.Server()), Via: m.via})
	}
	return out, mode == clientstate.RelayOnly
}

func (p *Pool) Hosts() []string {
	var out []string
	for _, m := range p.members() {
		out = append(out, m.dialer.host())
	}
	return out
}

const (
	reachWait = 4 * time.Second
	shakeWait = 8 * time.Second
)

func (p *Pool) Probe(endpoints []string) {
	if !p.probing.CompareAndSwap(false, true) {
		return
	}
	defer p.probing.Store(false)
	var wg sync.WaitGroup
	for _, m := range p.members() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.probe(m, endpoints)
		}()
	}
	wg.Wait()
}

func (p *Pool) probe(m *member, endpoints []string) {
	s := m.stat
	defer func() { s.checked.Store(time.Now().UnixMilli()) }()
	note := func(err error) {
		if err == nil {
			s.fault.Store(nil)
			return
		}
		text := err.Error()
		s.fault.Store(&text)
	}

	dialer := net.Dialer{Timeout: reachWait}
	if p.Keep != nil {
		dialer.Control = func(_, _ string, c syscall.RawConn) error { return c.Control(p.Keep) }
	}
	began := time.Now()
	conn, err := dialer.Dial("tcp", m.dialer.Target())
	if err != nil {
		s.reach.Store(-1)
		s.shake.Store(-1)
		note(err)
		return
	}
	conn.Close()
	s.reach.Store(max(1, time.Since(began).Milliseconds()))

	if len(endpoints) == 0 {
		s.shake.Store(0)
		note(fmt.Errorf("no entrypoint to reach through the relay"))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), shakeWait)
	defer cancel()
	began = time.Now()
	held, cc, err := cip.ReachH2(ctx, endpoints[rand.IntN(len(endpoints))], nil, m.via)
	if err != nil {
		s.shake.Store(-1)
		note(err)
		return
	}
	s.shake.Store(max(1, time.Since(began).Milliseconds()))
	cc.Close()
	held.Close()
	note(nil)
}

func (p *Pool) Follow(endpoints func() []string) {
	time.Sleep(5 * time.Second)
	p.Probe(endpoints())
	for range time.Tick(pace.ProbeEvery) {
		p.Probe(endpoints())
	}
}

func (p *Pool) States() map[int]clientapi.RelayState {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := map[int]clientapi.RelayState{}
	for id, s := range p.stats {
		state := clientapi.RelayState{
			Reach: s.reach.Load(), Shake: s.shake.Load(),
			Checked: s.checked.Load(),
		}
		if fault := s.fault.Load(); fault != nil {
			state.Fault = *fault
		}
		out[id] = state
	}
	for id, why := range p.broken {
		out[id] = clientapi.RelayState{Fault: why, Broken: true}
	}
	return out
}

func (p *Pool) Vias() ([]cip.Via, bool) {
	list, only := p.Now()
	out := make([]cip.Via, 0, len(list))
	for _, r := range list {
		out = append(out, r.Via)
	}
	return out, only
}

func (p *Pool) Renew() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, m := range p.open {
		d, err := p.dial(m.link, m.dialer.At)
		if err != nil {
			continue
		}
		m.dialer.Close()
		p.open[id] = &member{id: id, link: m.link, dialer: d, stat: m.stat}
	}
}
