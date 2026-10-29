package qdmobile

import (
	"hash/maphash"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/qd/internal/clientstate"
)

type marks struct {
	host Host

	mu   sync.RWMutex
	role map[string]string
	def  string

	flows sync.Map
	ports sync.Map
	live  atomic.Int64
	seed  maphash.Seed
}

const (
	flowCap  = 4096
	flowIdle = 5 * time.Minute

	portFresh = 10 * time.Second
	unsureFor = time.Second
)

type held struct {
	role string
	own  bool
	at   int64
	sure bool
}

func newMarks(host Host) *marks {
	return &marks{host: host, role: map[string]string{}, def: clientstate.RoleTunnel, seed: maphash.MakeSeed()}
}

func (m *marks) reload(db *clientstate.DB) {
	def, rules, err := db.RulesInForce()
	if err != nil {
		return
	}

	fresh := map[string]string{}
	for _, rule := range rules {
		if name := strings.TrimSpace(rule.Process); name != "" {
			fresh[name] = rule.Role
		}
	}

	m.mu.Lock()
	m.role, m.def = fresh, def
	m.mu.Unlock()

	m.forget()
}

func (m *marks) forget() {
	m.flows.Range(func(key, _ any) bool {
		m.flows.Delete(key)
		return true
	})
	m.ports.Clear()
	m.live.Store(0)
}

func (m *marks) global() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.def
}

func (m *marks) some() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.role) > 0
}

func (m *marks) forFlow(src, dst netip.AddrPort, udp bool) (string, bool) {
	if !m.some() || !src.IsValid() || !dst.IsValid() {
		return "", false
	}

	proto := byte(6)
	if udp {
		proto = 17
	}

	now := time.Now().UnixMilli()
	key := m.key(proto, src.Port(), dst.Addr())
	kept, known := m.flows.Load(key)
	if known {
		if was := kept.(held); was.sure || now-was.at < unsureFor.Milliseconds() {
			return was.role, was.own
		}
	}

	var fresh held
	if recent, seen := m.ports.Load(src.Port()); udp && seen && now-recent.(held).at < portFresh.Milliseconds() {
		fresh = recent.(held)
	} else if fresh = m.owner(proto, src, dst); fresh.sure && udp {
		fresh.at = now
		m.ports.Store(src.Port(), fresh)
	}
	fresh.at = now

	if known {
		m.flows.Store(key, fresh)
	} else {
		m.remember(key, fresh)
	}
	return fresh.role, fresh.own
}

func (m *marks) key(proto byte, sourcePort uint16, target netip.Addr) uint64 {
	var h maphash.Hash
	h.SetSeed(m.seed)
	h.WriteByte(proto)
	h.WriteByte(byte(sourcePort >> 8))
	h.WriteByte(byte(sourcePort))
	raw := target.As16()
	h.Write(raw[:])
	return h.Sum64()
}

func (m *marks) owner(proto byte, src, dst netip.AddrPort) held {
	started := time.Now()
	named := m.host.Owner(int(proto),
		src.Addr().String(), int(src.Port()),
		dst.Addr().String(), int(dst.Port()))
	if spent := time.Since(started).Milliseconds(); spent > 20 {
		say("owner: lookup took %d ms for proto=%d port=%d", spent, proto, src.Port())
	}
	if named == "" {
		return held{}
	}
	m.mu.RLock()
	role, own := m.role[named]
	m.mu.RUnlock()
	return held{role: role, own: own, sure: true}
}

func (m *marks) remember(key uint64, fresh held) {
	if _, already := m.flows.LoadOrStore(key, fresh); already {
		return
	}
	if m.live.Add(1) > flowCap {
		m.evict()
	}
}

func (m *marks) evict() {
	cutoff := time.Now().Add(-flowIdle).UnixMilli()
	m.flows.Range(func(key, value any) bool {
		if value.(held).at < cutoff {
			m.flows.Delete(key)
			m.live.Add(-1)
		}
		return true
	})

	if m.live.Load() <= flowCap {
		return
	}
	m.flows.Range(func(key, _ any) bool {
		m.flows.Delete(key)
		m.live.Add(-1)
		return m.live.Load() > flowCap/2
	})
}
