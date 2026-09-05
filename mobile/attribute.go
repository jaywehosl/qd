package qdmobile

import (
	"hash/maphash"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
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
	live  atomic.Int64
	seed  maphash.Seed
}

const (
	flowCap  = 4096
	flowIdle = 5 * time.Minute
)

type held struct {
	mark byte
	at   int64
}

func newMarks(host Host) *marks {
	return &marks{host: host, role: map[string]byte{}, seed: maphash.MakeSeed()}
}

func (m *marks) reload(db *clientstate.DB) {
	rules, err := db.Rules()
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
	m.live.Store(0)
}

func (m *marks) interested() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.role) > 0
}

// forFlow отвечает, каким выходом идти этому флоу. Спросить систему, кто хозяин
// флоу, можно только полной четвёркой адресов, и стоит это десятки миллисекунд:
// для соединения (wait) платим один раз, для датаграмм отвечаем из памяти, а
// хозяина узнаём в стороне — до ответа пакеты идут общим выходом.
func (m *marks) forFlow(src, dst netip.AddrPort, udp bool, wait bool) qdcrypt.Exit {
	global := qdcrypt.Exit(exit.Load())
	if !m.interested() || !src.IsValid() || !dst.IsValid() {
		return global
	}

	proto := byte(6)
	if udp {
		proto = 17
	}

	key := m.key(proto, src.Port(), dst.Addr())
	if kept, known := m.flows.Load(key); known {
		return exitOf(kept.(held).mark, global)
	}

	if !wait {
		go m.remember(key, m.owner(proto, src, dst))
		return global
	}

	mark := m.owner(proto, src, dst)
	m.remember(key, mark)
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

func (m *marks) owner(proto byte, src, dst netip.AddrPort) byte {
	started := time.Now()
	named := m.host.Owner(int(proto),
		src.Addr().String(), int(src.Port()),
		dst.Addr().String(), int(dst.Port()))
	if spent := time.Since(started).Milliseconds(); spent > 20 {
		say("owner: lookup took %d ms for proto=%d port=%d", spent, proto, src.Port())
	}
	if named == "" {
		return markGlobal
	}

	m.mu.RLock()
	mark, known := m.role[named]
	m.mu.RUnlock()
	if !known {
		return markGlobal
	}
	return mark
}

func (m *marks) remember(key uint64, mark byte) {
	if _, already := m.flows.LoadOrStore(key, held{mark: mark, at: time.Now().UnixMilli()}); already {
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
