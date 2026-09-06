// Package ippkt — разбор заголовка сырого IP-пакета: адреса, протокол, порты.
//
// Одно место намеренно. Тот же разбор был переписан в клиенте, в движке, на
// узле и в перехвате — пять раз с чуть разными границами длины, и правка в
// одном месте не доезжала до остальных.
//
// Ограничение общее для всех: IPv6 extension headers не разбираются, L4
// считается сразу после 40-байтного заголовка.
package ippkt

import (
	"encoding/binary"
	"net/netip"
)

const (
	protoTCP = 6
	protoUDP = 17
)

// Dst — адрес назначения.
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

// IsTCP — несёт ли пакет TCP.
func IsTCP(pkt []byte) bool {
	if len(pkt) < 1 {
		return false
	}
	switch pkt[0] >> 4 {
	case 4:
		return len(pkt) >= 20 && pkt[9] == protoTCP
	case 6:
		return len(pkt) >= 40 && pkt[6] == protoTCP
	}
	return false
}

// after отдаёт протокол и то, что идёт за IP-заголовком, если хвост не короче
// need байт.
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

// Flow — обе стороны разговора и его вид. ok=false для всего, что не TCP и не
// UDP: маршрутизировать такое по портам всё равно нельзя.
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

// SrcPort — порт источника.
func SrcPort(pkt []byte) (uint16, bool) {
	proto, rest, ok := after(pkt, 4)
	if !ok || (proto != protoTCP && proto != protoUDP) {
		return 0, false
	}
	return binary.BigEndian.Uint16(rest[0:2]), true
}

// IsDNS — UDP на 53-й порт.
func IsDNS(pkt []byte) bool {
	proto, rest, ok := after(pkt, 8)
	return ok && proto == protoUDP && binary.BigEndian.Uint16(rest[2:4]) == 53
}
