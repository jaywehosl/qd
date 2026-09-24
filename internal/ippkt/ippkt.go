package ippkt

import (
	"encoding/binary"
	"net/netip"
)

const (
	protoTCP = 6
	protoUDP = 17
)

func Dst(pkt []byte) (netip.Addr, bool) {
	if len(pkt) < 1 {
		return netip.Addr{}, false
	}
	switch pkt[0] >> 4 {
	case 4:
		if len(pkt) < 20 {
			return netip.Addr{}, false
		}
		return netip.AddrFrom4([4]byte(pkt[16:20])), true
	case 6:
		if len(pkt) < 40 {
			return netip.Addr{}, false
		}
		return netip.AddrFrom16([16]byte(pkt[24:40])), true
	}
	return netip.Addr{}, false
}

func IsTCP(pkt []byte) bool {
	proto, _, ok := after(pkt, 0)
	return ok && proto == protoTCP
}

func after(pkt []byte, need int) (byte, []byte, bool) {
	switch {
	case len(pkt) >= 20 && pkt[0]>>4 == 4:
		head := int(pkt[0]&0x0f) * 4
		if head < 20 || len(pkt) < head+need {
			return 0, nil, false
		}
		return pkt[9], pkt[head:], true
	case len(pkt) >= 40+need && pkt[0]>>4 == 6:
		return pkt[6], pkt[40:], true
	}
	return 0, nil, false
}

func Flow(pkt []byte) (src, dst netip.AddrPort, udp bool, ok bool) {
	proto, rest, ok := after(pkt, 4)
	if !ok || (proto != protoTCP && proto != protoUDP) {
		return src, dst, false, false
	}

	var from, to netip.Addr
	if pkt[0]>>4 == 4 {
		from = netip.AddrFrom4([4]byte(pkt[12:16]))
		to = netip.AddrFrom4([4]byte(pkt[16:20]))
	} else {
		from = netip.AddrFrom16([16]byte(pkt[8:24]))
		to = netip.AddrFrom16([16]byte(pkt[24:40]))
	}
	return netip.AddrPortFrom(from, binary.BigEndian.Uint16(rest[0:2])),
		netip.AddrPortFrom(to, binary.BigEndian.Uint16(rest[2:4])), proto == protoUDP, true
}

func SrcPort(pkt []byte) (uint16, bool) {
	proto, rest, ok := after(pkt, 4)
	if !ok || (proto != protoTCP && proto != protoUDP) {
		return 0, false
	}
	return binary.BigEndian.Uint16(rest[0:2]), true
}

func IsDNS(pkt []byte) bool {
	proto, rest, ok := after(pkt, 8)
	return ok && (proto == protoUDP || proto == protoTCP) && binary.BigEndian.Uint16(rest[2:4]) == 53
}

func Checksum(b []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(b[i:]))
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum > 0xFFFF {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}
	return ^uint16(sum)
}
