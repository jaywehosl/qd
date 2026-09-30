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

func (p *pool) takeOwn(session uint32) (netip.Prefix, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.own(session)
}

func (p *pool) own(session uint32) (netip.Prefix, bool) {
	was, ok := p.mine[session]
	if !ok {
		return netip.Prefix{}, false
	}
	if _, busy := p.taken[was]; busy || p.streams.Contains(was) {
		return netip.Prefix{}, false
	}
	p.taken[was] = struct{}{}
	return netip.PrefixFrom(was, was.BitLen()), true
}

func (p *pool) take(session uint32) (netip.Prefix, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if own, ok := p.own(session); ok {
		return own, nil
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

func sixOf(p netip.Prefix) netip.Prefix {
	v4 := p.Addr().As4()
	addr := netip.AddrFrom16([16]byte{0xfd, 0x00, 0x00, 0x07, 12: v4[0], v4[1], v4[2], v4[3]})
	return netip.PrefixFrom(addr, addr.BitLen())
}

func carried(address netip.Prefix) []netip.Prefix {
	if !HoldsV6() || !address.Addr().Is4() {
		return []netip.Prefix{address}
	}
	return []netip.Prefix{address, sixOf(address)}
}
