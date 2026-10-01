//go:build windows || linux

package main

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/qd/internal/clientstate"
)

const (
	ownerTTL   = 5 * time.Second
	unknownTTL = time.Second
	ownerKeep  = 10 * time.Second
	tidyEvery  = 30 * time.Second
)

type stats struct {
	procMiss atomic.Uint64
	lastMiss atomic.Pointer[string]
}

var st stats

type portKey struct {
	proto uint8
	v6    bool
	port  uint16
}

type procIdent struct {
	name string
	path string
}

type owner struct {
	pid      uint32
	bound    procIdent
	closed   bool
	closedAt int64

	decided bool
	by      uint32
	ident   procIdent
	until   int64
}

type procRouter struct {
	mu     sync.RWMutex
	byPath map[string]string
	byName map[string]string
	def    string

	prevByPath map[string]string
	prevByName map[string]string
	prevDef    string

	active atomic.Bool
	fixed  atomic.Pointer[string]

	owners sync.Map
	plat   platformRouter
}

func newProcRouter() *procRouter {
	return &procRouter{
		byPath: map[string]string{},
		byName: map[string]string{},
		def:    clientstate.RoleTunnel,
	}
}

func (r *procRouter) Load(defaultRole string, rules []clientstate.Rule) {
	byPath := map[string]string{}
	byName := map[string]string{}
	for _, rule := range rules {
		if rule.Path != "" {
			byPath[strings.ToLower(rule.Path)] = rule.Role
			continue
		}
		byName[strings.ToLower(rule.Process)] = rule.Role
	}
	if !clientstate.ValidRole(defaultRole) {
		defaultRole = clientstate.RoleTunnel
	}

	r.mu.Lock()
	r.prevByPath, r.prevByName, r.prevDef = r.byPath, r.byName, r.def
	r.byPath, r.byName, r.def = byPath, byName, defaultRole
	r.mu.Unlock()

	if len(byPath) == 0 && len(byName) == 0 {
		held := defaultRole
		r.fixed.Store(&held)
		r.active.Store(defaultRole != clientstate.RoleTunnel)
		return
	}
	r.fixed.Store(nil)
	r.active.Store(true)
}

func (r *procRouter) Active() bool { return r.active.Load() }

func (r *procRouter) RoleFor(pkt []byte) string {
	if held := r.fixed.Load(); held != nil {
		return *held
	}
	key, src, flags, ok := flowOf(pkt)
	if !ok {
		return r.fallback()
	}
	return r.roleOf(r.ownerOf(key, src, flags&(tcpSyn|tcpAck) == tcpSyn, flags&tcpRst != 0))
}

func (r *procRouter) RoleForFlow(proto uint8, src netip.AddrPort) string {
	if held := r.fixed.Load(); held != nil {
		return *held
	}
	addr := src.Addr().Unmap()
	return r.roleOf(r.ownerOf(portKey{proto: proto, v6: addr.Is6(), port: src.Port()}, addr, false, false))
}

func (r *procRouter) roleOf(ident procIdent, known bool) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !known {
		return r.def
	}
	return roleIn(ident, r.byPath, r.byName, r.def)
}

func (r *procRouter) ownerOf(key portKey, src netip.Addr, opens, resets bool) (procIdent, bool) {
	now := time.Now().UnixMilli()
	held, seen := r.owners.Load(key)
	var o owner
	if seen {
		o = held.(owner)
		if o.decided && !opens && now < o.until {
			return o.ident, o.ident.name != ""
		}
	}

	pid, found, read := r.pidOf(key, src)
	switch {
	case !read && o.decided:
	case found:
		ident := identOf(pid)
		if ident.name != "" || !o.decided || o.by != pid {
			o.ident = ident
		}
		o.by = pid
	default:
		r.flush()
		if again, still := r.owners.Load(key); still {
			held, seen, o = again, true, again.(owner)
		}
		o.by, o.ident = o.pid, o.bound
	}

	o.decided, o.until = true, now+ownerTTL.Milliseconds()
	if o.ident.name == "" {
		o.until = now + unknownTTL.Milliseconds()
		if !resets {
			missed(key, o.by)
		}
	}
	if seen {
		r.owners.CompareAndSwap(key, held, o)
	} else {
		r.owners.LoadOrStore(key, o)
	}
	return o.ident, o.ident.name != ""
}

func (r *procRouter) tidy() {
	for range time.Tick(tidyEvery) {
		cut := time.Now().Add(-ownerKeep).UnixMilli()
		r.owners.Range(func(key, held any) bool {
			o := held.(owner)
			if o.until < cut && (o.pid == 0 || (o.closed && o.closedAt < cut)) {
				r.owners.CompareAndDelete(key, held)
			}
			return true
		})
	}
}

func identOf(pid uint32) procIdent {
	ident := lookupProcess(pid)
	return procIdent{name: strings.ToLower(ident.name), path: strings.ToLower(ident.path)}
}

func identMemo() func(pid uint32) (procIdent, bool) {
	seen := map[uint32]procIdent{}
	return func(pid uint32) (procIdent, bool) {
		ident, ok := seen[pid]
		if !ok {
			ident = lookupProcess(pid)
			seen[pid] = ident
		}
		return ident, ident.name != ""
	}
}

func missed(key portKey, pid uint32) {
	proto := "tcp"
	if key.proto == protoUDP {
		proto = "udp"
	}
	what := fmt.Sprintf("%s port %d, no process holds it", proto, key.port)
	if pid != 0 {
		what = fmt.Sprintf("%s port %d, process %d gave no name", proto, key.port, pid)
	}
	st.lastMiss.Store(&what)
	st.procMiss.Add(1)
}

func (r *procRouter) fallback() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.def
}

const (
	tcpSyn = 0x02
	tcpRst = 0x04
	tcpAck = 0x10
)

func flowOf(pkt []byte) (key portKey, src netip.Addr, flags byte, ok bool) {
	if len(pkt) < 20 {
		return portKey{}, netip.Addr{}, 0, false
	}
	var proto byte
	var rest []byte
	switch pkt[0] >> 4 {
	case 4:
		ihl := int(pkt[0]&0x0F) * 4
		if ihl < 20 || len(pkt) < ihl+4 {
			return portKey{}, netip.Addr{}, 0, false
		}
		proto, rest = pkt[9], pkt[ihl:]
		src = netip.AddrFrom4([4]byte(pkt[12:16]))
	case 6:
		if len(pkt) < 44 {
			return portKey{}, netip.Addr{}, 0, false
		}
		proto, rest = pkt[6], pkt[40:]
		src = netip.AddrFrom16([16]byte(pkt[8:24]))
	default:
		return portKey{}, netip.Addr{}, 0, false
	}
	if proto != protoTCP && proto != protoUDP {
		return portKey{}, netip.Addr{}, 0, false
	}
	if proto == protoTCP && len(rest) > 13 {
		flags = rest[13]
	}
	return portKey{proto: proto, v6: src.Is6(), port: binary.BigEndian.Uint16(rest[0:2])}, src, flags, true
}

func (r *procRouter) tellMisses() {
	was := uint64(0)
	for range time.Tick(30 * time.Second) {
		now := st.procMiss.Load()
		if now == was {
			continue
		}
		last := ""
		if held := st.lastMiss.Load(); held != nil {
			last = *held
		}
		fmt.Printf("routing  %d flows went by the default role, their owner is not known (the last one: %s)\n", now-was, last)
		was = now
	}
}

func reloadProcessRules(db *clientstate.DB) {
	if db == nil {
		return
	}
	r := routeByProcess.Load()
	if r == nil {
		r = newProcRouter()
		r.start()
		routeByProcess.Store(r)
		go r.tidy()
		go r.tellMisses()
	}
	def, rules, err := db.RulesInForce()
	if err != nil {
		return
	}
	r.Load(def, rules)
	splitRules(r)

	if n := r.dropRerouted(); n > 0 {
		fmt.Printf("routing  %d connections dropped so the new rule takes hold now\n", n)
	}
	if held := liveTunnel.Load(); held != nil {
		(*held).Reroute()
	}
}

func roleIn(ident procIdent, byPath, byName map[string]string, def string) string {
	if ident.path != "" {
		if role, ok := byPath[strings.ToLower(ident.path)]; ok {
			return role
		}
	}
	if role, ok := byName[strings.ToLower(ident.name)]; ok {
		return role
	}
	return def
}
