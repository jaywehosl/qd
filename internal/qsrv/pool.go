package qsrv

import (
	"errors"
	"net/netip"
	"sync"
)

var errPoolFull = errors.New("qsrv: no address left in the pool")

type pool struct {
	mu      sync.Mutex
	base    netip.Prefix
	next    netip.Addr
	taken   map[netip.Addr]struct{}
	mine    map[uint32]netip.Addr
	streams netip.Prefix
}

func newPool(prefix netip.Prefix) *pool {
	return &pool{
		base:    prefix,
		next:    prefix.Addr().Next(),
		taken:   map[netip.Addr]struct{}{},
		mine:    map[uint32]netip.Addr{},
		streams: streamTail(prefix),
	}
}

func streamTail(base netip.Prefix) netip.Prefix {
	if !base.Addr().Is4() || base.Bits() > 24 {
		return netip.Prefix{}
	}
	raw := base.Masked().Addr().As4()
	first := uint32(raw[0])<<24 | uint32(raw[1])<<16 | uint32(raw[2])<<8 | uint32(raw[3])
	span := uint32(1)<<uint(32-base.Bits()) - 1
	top := (first | span) &^ 0xFF
	return netip.PrefixFrom(netip.AddrFrom4([4]byte{
		byte(top >> 24), byte(top >> 16), byte(top >> 8), byte(top),
	}), 24)
}

func (p *pool) stream(seat uint32) netip.Prefix {
	if !p.streams.IsValid() {
		addr := netip.AddrFrom4([4]byte{10, 7, 255, 254})
		return netip.PrefixFrom(addr, addr.BitLen())
	}
	raw := p.streams.Addr().As4()
	raw[3] = byte(seat%254) + 1
	addr := netip.AddrFrom4(raw)
	return netip.PrefixFrom(addr, addr.BitLen())
}

func (p *pool) take(session uint32) (netip.Prefix, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if was, ok := p.mine[session]; ok {
		if _, busy := p.taken[was]; !busy && !p.streams.Contains(was) {
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
		if p.streams.IsValid() && p.streams.Contains(addr) {
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
	return netip.Prefix{}, errPoolFull
}

func (p *pool) give(prefix netip.Prefix) {
	p.mu.Lock()
	delete(p.taken, prefix.Addr())
	p.mu.Unlock()
}
