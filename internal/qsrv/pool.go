package qsrv

import (
	"errors"
	"net/netip"
	"sync"
)

var ErrPoolFull = errors.New("qsrv: no address left in the pool")

type pool struct {
	mu    sync.Mutex
	base  netip.Prefix
	next  netip.Addr
	taken map[netip.Addr]struct{}
	mine  map[uint32]netip.Addr
}

func newPool(prefix netip.Prefix) *pool {
	first := prefix.Addr().Next()
	return &pool{
		base:  prefix,
		next:  first,
		taken: map[netip.Addr]struct{}{},
		mine:  map[uint32]netip.Addr{},
	}
}

func (p *pool) take(session uint32) (netip.Prefix, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if was, ok := p.mine[session]; ok {
		if _, busy := p.taken[was]; !busy {
			p.taken[was] = struct{}{}
			return netip.PrefixFrom(was, was.BitLen()), nil
		}
	}

	for i := 0; i < 1<<20; i++ {
		addr := p.next
		p.next = p.next.Next()
		if !p.base.Contains(addr) {
			p.next = p.base.Addr().Next()
			continue
		}
		if _, busy := p.taken[addr]; busy {
			continue
		}
		p.taken[addr] = struct{}{}
		if session != 0 {
			p.mine[session] = addr
		}
		return netip.PrefixFrom(addr, addr.BitLen()), nil
	}
	return netip.Prefix{}, ErrPoolFull
}
func (p *pool) give(prefix netip.Prefix) {
	p.mu.Lock()
	delete(p.taken, prefix.Addr())
	p.mu.Unlock()
}

func (p *pool) held() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.taken)
}
