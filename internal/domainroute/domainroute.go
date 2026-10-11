package domainroute

import (
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	keep = time.Hour
	room = 1 << 16
)

type Entry struct {
	Rule string
	Role string
}

type Net struct {
	Prefix netip.Prefix
	Entry
}

type spans struct {
	entry  Entry
	lo, hi []netip.Addr
}

const fewNets = 64

func last(p netip.Prefix) netip.Addr {
	raw := p.Masked().Addr().AsSlice()
	for bit := p.Bits(); bit < len(raw)*8; bit++ {
		raw[bit/8] |= 1 << (7 - bit%8)
	}
	out, _ := netip.AddrFromSlice(raw)
	return out
}

func (s spans) has(a netip.Addr) bool {
	i := sort.Search(len(s.lo), func(i int) bool { return s.lo[i].Compare(a) > 0 }) - 1
	return i >= 0 && s.lo[i].Is4() == a.Is4() && s.hi[i].Compare(a) >= 0
}

func sortNets(nets []Net) (few []Net, many []spans) {
	byRule := map[string][]Net{}
	var order []string
	for _, n := range nets {
		if _, seen := byRule[n.Rule+n.Role]; !seen {
			order = append(order, n.Rule+n.Role)
		}
		byRule[n.Rule+n.Role] = append(byRule[n.Rule+n.Role], n)
	}
	for _, key := range order {
		group := byRule[key]
		if len(group) <= fewNets {
			few = append(few, group...)
			continue
		}
		sort.Slice(group, func(i, j int) bool { return group[i].Prefix.Addr().Compare(group[j].Prefix.Addr()) < 0 })
		set := spans{entry: group[0].Entry}
		for _, n := range group {
			set.lo = append(set.lo, n.Prefix.Masked().Addr())
			set.hi = append(set.hi, last(n.Prefix))
		}
		many = append(many, set)
	}
	return few, many
}

type learned struct {
	rule  string
	role  string
	low   bool
	until int64
}

type answer struct {
	addrs []netip.Addr
	until int64
}

type Table struct {
	active atomic.Bool

	Placed func(role string, addrs []netip.Addr)

	mu    sync.RWMutex
	rules map[string]string
	lower map[string]Entry
	few   []Net
	many  []spans
	names map[string]answer
	addrs map[netip.Addr]learned
}

func New() *Table {
	return &Table{rules: map[string]string{}, lower: map[string]Entry{}, names: map[string]answer{}, addrs: map[netip.Addr]learned{}}
}

func (t *Table) Load(rules map[string]string, lower map[string]Entry, nets []Net) {
	now := time.Now().Unix()
	few, many := sortNets(nets)
	t.mu.Lock()
	t.rules, t.lower, t.few, t.many = rules, lower, few, many
	t.addrs = map[netip.Addr]learned{}
	for name, held := range t.names {
		if held.until < now {
			delete(t.names, name)
			continue
		}
		t.place(name, held)
	}
	t.mu.Unlock()
	t.active.Store(len(rules)+len(lower)+len(nets) > 0)
}

func (t *Table) Active() bool { return t.active.Load() }

func tails(name string, each func(tail string) bool) {
	for name != "" {
		if each(name) {
			return
		}
		dot := strings.IndexByte(name, '.')
		if dot < 0 {
			return
		}
		name = name[dot+1:]
	}
}

func (t *Table) ruleOf(name string) (found learned, ok bool) {
	tails(name, func(tail string) bool {
		if role, held := t.rules[tail]; held {
			found, ok = learned{rule: tail, role: role}, true
		}
		return ok
	})
	if ok {
		return found, true
	}
	tails(name, func(tail string) bool {
		if entry, held := t.lower[tail]; held {
			found, ok = learned{rule: entry.Rule, role: entry.Role, low: true}, true
		}
		return ok
	})
	return found, ok
}

func (t *Table) RoleOfName(name string) (string, bool) {
	if !t.active.Load() {
		return "", false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	found, ok := t.ruleOf(strings.Trim(strings.ToLower(name), "."))
	return found.role, ok
}

func (t *Table) held(addr netip.Addr) (learned, bool) {
	if !t.active.Load() {
		return learned{}, false
	}
	addr = addr.Unmap()
	t.mu.RLock()
	defer t.mu.RUnlock()
	if held, ok := t.addrs[addr]; ok && held.until >= time.Now().Unix() {
		return held, true
	}
	for _, n := range t.few {
		if n.Prefix.Contains(addr) {
			return learned{rule: n.Rule, role: n.Role, low: true}, true
		}
	}
	for _, set := range t.many {
		if set.has(addr) {
			return learned{rule: set.entry.Rule, role: set.entry.Role, low: true}, true
		}
	}
	return learned{}, false
}

func (t *Table) RoleOf(addr netip.Addr) (string, bool) {
	held, ok := t.held(addr)
	return held.role, ok
}

func (t *Table) RuleOf(addr netip.Addr) (string, bool) {
	held, ok := t.held(addr)
	return held.rule, ok
}

func (t *Table) place(name string, got answer) {
	found, ok := t.ruleOf(name)
	if !ok {
		return
	}
	found.until = got.until
	now := time.Now().Unix()
	for _, a := range got.addrs {
		a = a.Unmap()
		if was, taken := t.addrs[a]; taken && found.low && !was.low && was.until >= now {
			continue
		}
		t.addrs[a] = found
	}
	if t.Placed != nil {
		t.Placed(found.role, got.addrs)
	}
}

func (t *Table) Replay() {
	now := time.Now().Unix()
	t.mu.Lock()
	defer t.mu.Unlock()
	for name, held := range t.names {
		if held.until >= now {
			t.place(name, held)
		}
	}
}

func (t *Table) Learn(name string, addrs []netip.Addr) {
	if len(addrs) == 0 {
		return
	}
	name = strings.Trim(strings.ToLower(name), ".")
	now := time.Now()
	got := answer{addrs: addrs, until: now.Add(keep).Unix()}

	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.names) > room {
		for n, held := range t.names {
			if held.until < now.Unix() {
				delete(t.names, n)
			}
		}
	}
	if len(t.addrs) > room {
		for a, held := range t.addrs {
			if held.until < now.Unix() {
				delete(t.addrs, a)
			}
		}
	}
	t.names[name] = got
	t.place(name, got)
}
