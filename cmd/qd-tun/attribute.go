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

	"github.com/jaywehosl/quic-diver/internal/clientstate"
)

const (
	tableTTL     = 5 * time.Second
	missCooldown = 40 * time.Millisecond

	flowCap  = 8192
	flowIdle = 5 * time.Minute
)

type stats struct {
	procMiss atomic.Uint64
}

var st stats

type portKey struct {
	proto uint8
	port  uint16
}

type flowKey struct {
	proto uint8
	port  uint16
	dst   netip.Addr
}

type procIdent struct {
	name string
	path string
}

type verdict struct {
	role string
	at   int64
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

	aside      sync.Map
	keptAside  atomic.Int64
	chosen     sync.Map
	keptChosen atomic.Int64

	watched sync.Map
	tbl     atomic.Pointer[map[portKey]uint32]
	refresh chan struct{}

	pidMu sync.Mutex
	pids  map[uint32]procIdent
}

func newProcRouter() *procRouter {
	return &procRouter{
		byPath:  map[string]string{},
		byName:  map[string]string{},
		def:     clientstate.RoleTunnel,
		refresh: make(chan struct{}, 1),
		pids:    map[uint32]procIdent{},
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

	r.forgetFlows()

	if len(byPath) == 0 && len(byName) == 0 {
		held := defaultRole
		r.fixed.Store(&held)
		r.active.Store(defaultRole != clientstate.RoleTunnel)
		return
	}
	r.fixed.Store(nil)
	r.active.Store(true)
}

func (r *procRouter) forgetFlows() {
	for _, one := range []struct {
		where *sync.Map
		count *atomic.Int64
	}{{&r.aside, &r.keptAside}, {&r.chosen, &r.keptChosen}} {
		one.where.Range(func(k, _ any) bool {
			one.where.Delete(k)
			return true
		})
		one.count.Store(0)
	}
}

func (r *procRouter) Active() bool { return r.active.Load() }

func (r *procRouter) RoleFor(pkt []byte) string {
	if held := r.fixed.Load(); held != nil {
		return *held
	}

	key, ok := flowOf(pkt)
	if !ok {
		return r.fallback()
	}
	if held, known := r.aside.Load(key); known {
		return held.(verdict).role
	}

	pid, known := r.pidFor(portKey{proto: key.proto, port: key.port})
	role, _ := r.roleOfPid(pid, known)
	r.keep(&r.aside, &r.keptAside, key, role)
	return role
}

func (r *procRouter) RoleForFlow(proto uint8, port uint16, dst netip.Addr) string {
	if held := r.fixed.Load(); held != nil {
		return *held
	}

	key := flowKey{proto: proto, port: port, dst: dst}
	if held, known := r.chosen.Load(key); known {
		return held.(verdict).role
	}

	pid, known := r.awaitOwner(portKey{proto: proto, port: port})
	role, sure := r.roleOfPid(pid, known)
	if sure {
		r.keep(&r.chosen, &r.keptChosen, key, role)
	}
	return role
}

func (r *procRouter) awaitOwner(key portKey) (uint32, bool) {
	for waited := time.Duration(0); ; waited += ownerStep {
		if pid, ok := r.pidFor(key); ok {
			return pid, true
		}
		if waited >= ownerPatience {
			return 0, false
		}
		time.Sleep(ownerStep)
	}
}

func (r *procRouter) keep(where *sync.Map, count *atomic.Int64, key flowKey, role string) {
	if _, already := where.LoadOrStore(key, verdict{role: role, at: time.Now().UnixMilli()}); already {
		return
	}
	if count.Add(1) > flowCap {
		r.evict(where, count)
	}
}

func (r *procRouter) roleOfPid(pid uint32, known bool) (string, bool) {
	if !known {
		st.procMiss.Add(1)
		return r.fallback(), false
	}
	ident, ok := r.identFor(pid)
	if !ok {
		st.procMiss.Add(1)
		return r.fallback(), false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.roleOf(ident, r.byPath, r.byName, r.def), true
}

const (
	ownerStep     = 2 * time.Millisecond
	ownerPatience = 60 * time.Millisecond
)

func (r *procRouter) evict(where *sync.Map, count *atomic.Int64) {
	cutoff := time.Now().Add(-flowIdle).UnixMilli()
	where.Range(func(key, held any) bool {
		if held.(verdict).at < cutoff {
			where.Delete(key)
			count.Add(-1)
		}
		return true
	})
	if count.Load() <= flowCap {
		return
	}
	where.Range(func(key, _ any) bool {
		where.Delete(key)
		count.Add(-1)
		return count.Load() > flowCap/2
	})
}

func (r *procRouter) fallback() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.def
}

func flowOf(pkt []byte) (flowKey, bool) {
	if len(pkt) < 20 {
		return flowKey{}, false
	}
	var proto byte
	var rest []byte
	var dst netip.Addr
	switch pkt[0] >> 4 {
	case 4:
		ihl := int(pkt[0]&0x0F) * 4
		if ihl < 20 || len(pkt) < ihl+4 {
			return flowKey{}, false
		}
		proto, rest = pkt[9], pkt[ihl:]
		dst = netip.AddrFrom4([4]byte(pkt[16:20]))
	case 6:
		if len(pkt) < 44 {
			return flowKey{}, false
		}
		proto, rest = pkt[6], pkt[40:]
		dst = netip.AddrFrom16([16]byte(pkt[24:40]))
	default:
		return flowKey{}, false
	}
	if proto != protoTCP && proto != protoUDP {
		return flowKey{}, false
	}
	return flowKey{proto: proto, port: binary.BigEndian.Uint16(rest[0:2]), dst: dst}, true
}

func (r *procRouter) pidFor(key portKey) (uint32, bool) {
	if pid, ok := r.watched.Load(key); ok {
		return pid.(uint32), true
	}

	held := r.tbl.Load()
	if held == nil {
		r.wake()
		return 0, false
	}
	pid, ok := (*held)[key]
	if !ok {
		r.wake()
	}
	return pid, ok
}

func (r *procRouter) wake() {
	select {
	case r.refresh <- struct{}{}:
	default:
	}
}

func (r *procRouter) tellMisses(stop <-chan struct{}) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()

	was := uint64(0)
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
		}
		now := st.procMiss.Load()
		if now > was {
			fmt.Printf("routing  %d flows went by the default role, their owner was not known in time\n", now-was)
			was = now
		}
	}
}

func (r *procRouter) keepTable(stop <-chan struct{}) {
	tick := time.NewTicker(tableTTL)
	defer tick.Stop()

	for {
		if r.fixed.Load() == nil {
			r.readTable()
		}

		select {
		case <-stop:
			return
		case <-tick.C:
		case <-r.refresh:
			time.Sleep(missCooldown)
		}
	}
}

func (r *procRouter) readTable() {
	next := make(map[portKey]uint32, 512)
	readSockets(next)
	if len(next) == 0 {
		return
	}
	r.tbl.Store(&next)
}

func (r *procRouter) identFor(pid uint32) (procIdent, bool) {
	if pid == 0 {
		return procIdent{}, false
	}

	r.pidMu.Lock()
	ident, ok := r.pids[pid]
	r.pidMu.Unlock()
	if ok {
		return ident, ident.name != ""
	}

	ident = lookupProcess(pid)
	r.pidMu.Lock()
	if len(r.pids) > 4096 {
		r.pids = map[uint32]procIdent{}
	}
	r.pids[pid] = ident
	r.pidMu.Unlock()
	return ident, ident.name != ""
}

func reloadProcessRules(db *clientstate.DB) {
	if db == nil {
		return
	}
	r := routeByProcess.Load()
	if r == nil {
		r = newProcRouter()
		routeByProcess.Store(r)
		go r.keepTable(nil)
		go r.tellMisses(nil)
	}
	rules, err := db.Rules()
	if err != nil {
		return
	}
	def, err := db.DefaultRole()
	if err != nil {
		def = clientstate.RoleTunnel
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

func (r *procRouter) roleOf(ident procIdent, byPath, byName map[string]string, def string) string {
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
