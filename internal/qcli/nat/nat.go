package nat

import (
	"encoding/binary"
	"net/netip"
)

type NAT struct {
	realV4, assignedV4 [4]byte
	haveV4             bool
	realV6, assignedV6 [16]byte
	haveV6             bool
}

func New(reals, assigned []netip.Addr) *NAT {
	n := &NAT{}
	realV4, realV6, okr4, okr6 := pick(reals)
	asgV4, asgV6, oka4, oka6 := pick(assigned)
	if okr4 && oka4 {
		n.realV4, n.assignedV4, n.haveV4 = realV4.As4(), asgV4.As4(), true
	}
	if okr6 && oka6 {
		n.realV6, n.assignedV6, n.haveV6 = realV6.As16(), asgV6.As16(), true
	}
	return n
}

func pick(addrs []netip.Addr) (v4, v6 netip.Addr, hasV4, hasV6 bool) {
	for _, a := range addrs {
		if a.Is4() && !hasV4 {
			v4, hasV4 = a, true
		} else if a.Is6() && !hasV6 {
			v6, hasV6 = a, true
		}
	}
	return
}

func (n *NAT) Outbound(pkt []byte) { n.apply(pkt, true) }

func (n *NAT) Inbound(pkt []byte) { n.apply(pkt, false) }

func (n *NAT) apply(pkt []byte, outbound bool) {
	if len(pkt) < 1 {
		return
	}
	switch pkt[0] >> 4 {
	case 4:
		n.applyV4(pkt, outbound)
	case 6:
		n.applyV6(pkt, outbound)
	}
}

func (n *NAT) applyV4(pkt []byte, outbound bool) {
	if !n.haveV4 || len(pkt) < 20 {
		return
	}
	var field []byte
	var from, to [4]byte
	if outbound {
		field = pkt[12:16]
		from, to = n.realV4, n.assignedV4
	} else {
		field = pkt[16:20]
		from, to = n.assignedV4, n.realV4
	}
	if [4]byte(field) != from {
		return
	}
	copy(field, to[:])
	fixIPv4Header(pkt)
	fixL4(pkt, 4, from[:], to[:])
}

func (n *NAT) applyV6(pkt []byte, outbound bool) {
	if !n.haveV6 || len(pkt) < 40 {
		return
	}
	var field []byte
	var from, to [16]byte
	if outbound {
		field = pkt[8:24]
		from, to = n.realV6, n.assignedV6
	} else {
		field = pkt[24:40]
		from, to = n.assignedV6, n.realV6
	}
	if [16]byte(field) != from {
		return
	}
	copy(field, to[:])
	fixL4(pkt, 6, from[:], to[:])
}

func fixIPv4Header(pkt []byte) {
	ihl := int(pkt[0]&0x0F) * 4
	if ihl < 20 || ihl > len(pkt) {
		return
	}
	pkt[10], pkt[11] = 0, 0
	var sum uint32
	for i := 0; i+1 < ihl; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(pkt[i:]))
	}
	for sum > 0xFFFF {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}
	binary.BigEndian.PutUint16(pkt[10:], ^uint16(sum))
}

func fixL4(pkt []byte, ver int, old, new []byte) {
	var l4off int
	var proto byte
	if ver == 4 {
		l4off = int(pkt[0]&0x0F) * 4
		proto = pkt[9]
	} else {
		l4off = 40
		proto = pkt[6]
	}

	var csumOff int
	switch proto {
	case 6:
		csumOff = l4off + 16
	case 17:
		csumOff = l4off + 6
	case 58:
		csumOff = l4off + 2
	default:
		return
	}
	if csumOff+2 > len(pkt) {
		return
	}

	c := binary.BigEndian.Uint16(pkt[csumOff:])
	if proto == 17 && ver == 4 && c == 0 {
		return
	}
	nc := csumUpdate(c, old, new)
	if proto == 17 && nc == 0 {
		nc = 0xFFFF
	}
	binary.BigEndian.PutUint16(pkt[csumOff:], nc)
}

func csumUpdate(hc uint16, old, new []byte) uint16 {
	sum := uint32(hc ^ 0xFFFF)
	for i := 0; i+1 < len(old); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(old[i:]) ^ 0xFFFF)
		sum += uint32(binary.BigEndian.Uint16(new[i:]))
	}
	for sum > 0xFFFF {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}
	return uint16(sum ^ 0xFFFF)
}
