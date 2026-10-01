package qdmobile

import (
	"hash/maphash"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/qdcrypt"
)

const (
	markGlobal = 1
	markEgress = 2
	markLocal  = 3
)

type marks struct {
	host Host

	mu   sync.RWMutex
	role map[string]byte

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
	mark byte
	at   int64
	sure bool
}

func newMarks(host Host) *marks {
	return &marks{host: host, role: map[string]byte{}, seed: maphash.MakeSeed()}
}

func (m *marks) reload(db *clientstate.DB) {
	_, rules, err := db.RulesInForce()
	if err != nil {
		return
	}

	fresh := map[string]byte{}
	for _, rule := range rules {
		name := strings.TrimSpace(rule.Process)
		if name == "" {
			continue
		}
		switch rule.Role {
		case clientstate.RoleEgress:
			fresh[name] = markEgress
		case clientstate.RoleNoEgress:
			fresh[name] = markLocal
		default:
			fresh[name] = markGlobal
		}
	}

	m.mu.Lock()
	m.role = fresh
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

func (m *marks) interested() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.role) > 0
}

func (m *marks) forFlow(src, dst netip.AddrPort, udp bool) qdcrypt.Exit {
	global := qdcrypt.Exit(exit.Load())
	if !m.interested() || !src.IsValid() || !dst.IsValid() {
		return global
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
			return exitOf(was.mark, global)
		}
	}

	var mark byte
	sure := false
	if recent, seen := m.ports.Load(src.Port()); udp && seen && now-recent.(held).at < portFresh.Milliseconds() {
		mark, sure = recent.(held).mark, true
	} else if mark, sure = m.owner(proto, src, dst); sure && udp {
		m.ports.Store(src.Port(), held{mark: mark, at: now})
	}

	fresh := held{mark: mark, at: now, sure: sure}
	if known {
		m.flows.Store(key, fresh)
	} else {
		m.remember(key, fresh)
	}
	return exitOf(mark, global)
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

func exitOf(mark byte, global qdcrypt.Exit) qdcrypt.Exit {
	switch mark {
	case markEgress:
		return qdcrypt.ExitEgress
	case markLocal:
		return qdcrypt.ExitLocal
	}
	return global
}

func (m *marks) owner(proto byte, src, dst netip.AddrPort) (byte, bool) {
	started := time.Now()
	named := m.host.Owner(int(proto),
		src.Addr().String(), int(src.Port()),
		dst.Addr().String(), int(dst.Port()))
	if spent := time.Since(started).Milliseconds(); spent > 20 {
		say("owner: lookup took %d ms for proto=%d port=%d", spent, proto, src.Port())
	}
	if named == "" {
		return markGlobal, false
	}

	m.mu.RLock()
	mark, known := m.role[named]
	m.mu.RUnlock()
	if !known {
		return markGlobal, true
	}
	return mark, true
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
