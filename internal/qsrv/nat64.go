package qsrv

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var synthetic = netip.MustParsePrefix("198.18.0.0/15")

const (
	natCeiling = 8192
	natIdle    = time.Hour
)

type pair struct {
	v6     netip.Addr
	usedAt atomic.Int64
}

type nat64 struct {
	marks atomic.Uint32
	mu    sync.RWMutex
	byV4  map[netip.Addr]*pair
	byV6  map[netip.Addr]netip.Addr
	next  uint32
}

func newNAT64() *nat64 {
	return &nat64{
		byV4: map[netip.Addr]*pair{},
		byV6: map[netip.Addr]netip.Addr{},
	}
}

func (n *nat64) stand(v6 netip.Addr) (netip.Addr, bool) {
	if !v6.Is6() || v6.Is4In6() {
		return netip.Addr{}, false
	}
	now := time.Now().Unix()

	n.mu.RLock()
	held, ok := n.byV6[v6]
	if ok {
		if p := n.byV4[held]; p != nil {
			p.usedAt.Store(now)
		}
	}
	n.mu.RUnlock()
	if ok {
		return held, true
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if held, ok := n.byV6[v6]; ok {
		return held, true
	}
	if len(n.byV4) >= natCeiling {
		n.forgetOldestLocked()
	}

	base := synthetic.Addr().As4()
	room := uint32(1)<<(32-synthetic.Bits()) - 2

	var v4 netip.Addr
	for i := uint32(0); i < room; i++ {
		n.next = (n.next + 1) % room
		raw := binary.BigEndian.Uint32(base[:]) + n.next + 1
		var out [4]byte
		binary.BigEndian.PutUint32(out[:], raw)
		v4 = netip.AddrFrom4(out)
		if _, taken := n.byV4[v4]; !taken {
			break
		}
	}

	p := &pair{v6: v6}
	p.usedAt.Store(now)
	n.byV6[v6] = v4
	n.byV4[v4] = p
	n.marks.Add(1)
	return v4, true
}

func (n *nat64) real(v4 netip.Addr) (netip.Addr, bool) {
	if !synthetic.Contains(v4) {
		return netip.Addr{}, false
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	held, ok := n.byV4[v4]
	if !ok {
		return netip.Addr{}, false
	}
	held.usedAt.Store(time.Now().Unix())
	return held.v6, true
}

func (n *nat64) forgetOldestLocked() {
	var oldest netip.Addr
	var since int64

	for v4, p := range n.byV4 {
		at := p.usedAt.Load()
		if since == 0 || at < since {
			oldest, since = v4, at
		}
	}
	if !oldest.IsValid() {
		return
	}
	if p := n.byV4[oldest]; p != nil {
		delete(n.byV6, p.v6)
	}
	delete(n.byV4, oldest)
}

func (n *nat64) sweep() int {
	cut := time.Now().Add(-natIdle).Unix()

	n.mu.Lock()
	defer n.mu.Unlock()

	gone := 0
	for v4, p := range n.byV4 {
		if p.usedAt.Load() > cut {
			continue
		}
		delete(n.byV6, p.v6)
		delete(n.byV4, v4)
		gone++
	}
	if gone > 0 {
		n.marks.Add(1)
	}
	return gone
}

func (n *Node) Stand(v6 netip.Addr) (netip.Addr, bool) {
	if n.nat == nil {
		return netip.Addr{}, false
	}
	return n.nat.stand(v6)
}

func (n *Node) behind(dst netip.AddrPort) netip.AddrPort {
	if n.nat == nil {
		return dst
	}
	if real, ok := n.nat.real(dst.Addr()); ok {
		return netip.AddrPortFrom(real, dst.Port())
	}
	return dst
}

func (n *Node) stale(dst netip.AddrPort) bool {
	if n.nat == nil || !synthetic.Contains(dst.Addr()) {
		return false
	}
	_, known := n.nat.real(dst.Addr())
	return !known
}

type natFile struct {
	Next  uint32            `json:"next"`
	Pairs map[string]string `json:"pairs"`
}

func (n *nat64) load(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var held natFile
	if json.Unmarshal(raw, &held) != nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	n.next = held.Next
	for v4text, v6text := range held.Pairs {
		v4, err4 := netip.ParseAddr(v4text)
		v6, err6 := netip.ParseAddr(v6text)
		if err4 != nil || err6 != nil {
			continue
		}
		p := &pair{v6: v6}
		p.usedAt.Store(time.Now().Unix())
		n.byV4[v4] = p
		n.byV6[v6] = v4
	}
}

func (n *nat64) save(path string) error {
	n.mu.RLock()
	held := natFile{Next: n.next, Pairs: make(map[string]string, len(n.byV4))}
	for v4, p := range n.byV4 {
		held.Pairs[v4.String()] = p.v6.String()
	}
	n.mu.RUnlock()

	raw, err := json.Marshal(held)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func (n *nat64) dirty() bool { return n.marks.Swap(0) > 0 }

func (n *Node) Remember(ctx context.Context, path string) {
	if n.nat == nil || path == "" {
		return
	}
	n.nat.load(path)

	tick := time.NewTicker(keepEvery)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			n.nat.save(path)
			return
		case <-tick.C:
			if gone := n.nat.sweep(); gone > 0 {
				n.cfg.Log("nat46     %d pairs went idle and were let go", gone)
			}
			if n.nat.dirty() {
				n.nat.save(path)
			}
		}
	}
}

const keepEvery = 20 * time.Second
