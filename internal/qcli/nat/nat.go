// Package nat — клиентский 1:1 адресный NAT для модели B.
//
// connect-ip (RFC 9484) требует, чтобы клиент слал пакеты с адреса, назначенного
// узлом (ADDRESS_ASSIGN). Приложения же шлют с реального LAN-адреса:
//   - outbound: src real → assigned;
//   - inbound:  dst assigned → real.
//
// Реальный адрес не фиксируется при старте: у машины бывает несколько адаптеров
// (виртуальные сети, второй интерфейс), и адрес меняется при переезде в другую
// сеть. Взятый однажды «первый» адрес переставал совпадать, пакеты уходили с
// настоящим src, и узел отбрасывал их как чужие. Поэтому src узнаётся из самого
// потока и обновляется, когда меняется.
//
// Порты не трогаются (не PAT). Контрольные суммы правятся инкрементально
// (RFC 1624): IPv4-заголовок пересчитывается, L4 (TCP/UDP/ICMPv6) — по дельте
// адреса в псевдо-заголовке.
//
// Ограничение: IPv6 extension headers не разбираются — L4 считается сразу после
// 40-байтного заголовка. Для TCP/UDP без ext-хедеров (обычный случай) корректно.
package nat

import (
	"encoding/binary"
	"net/netip"
	"sync/atomic"
)

// NAT хранит соответствие real↔assigned по семействам.
type NAT struct {
	assignedV4 [4]byte
	haveV4     bool
	assignedV6 [16]byte
	haveV6     bool

	realV4 atomic.Pointer[[4]byte]
	realV6 atomic.Pointer[[16]byte]
}

// New строит NAT под адреса, назначенные узлом. Реальный адрес клиента здесь не
// задаётся: его узнаёт Outbound из самого потока.
//
// Брать его из адресов интерфейсов нельзя, и это стоило потерянных пакетов. На
// телефоне устройство поднято уже под назначенным адресом, подменять нечего — а
// «первым адресом интерфейса» оказывался адрес Wi-Fi, и обратный путь уводил
// ответы на него. Пакет с чужим адресом назначения система отправляла обратно в
// туннель по маршруту по умолчанию, узел видел ответ сервера как исходящий
// пакет клиента и отбрасывал его: «source address not allowed».
func New(assigned []netip.Addr) *NAT {
	n := &NAT{}
	asgV4, asgV6, oka4, oka6 := pick(assigned)
	if oka4 {
		n.assignedV4, n.haveV4 = asgV4.As4(), true
	}
	if oka6 {
		n.assignedV6, n.haveV6 = asgV6.As16(), true
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

// Outbound переписывает src real→assigned на месте.
func (n *NAT) Outbound(pkt []byte) { n.apply(pkt, true) }

// Inbound переписывает dst assigned→real на месте.
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

	if outbound {
		src := [4]byte(pkt[12:16])
		if src == n.assignedV4 {
			return
		}
		if held := n.realV4.Load(); held == nil || *held != src {
			n.realV4.Store(&src)
		}
		copy(pkt[12:16], n.assignedV4[:])
		fixIPv4Header(pkt)
		fixL4(pkt, 4, src[:], n.assignedV4[:])
		return
	}

	held := n.realV4.Load()
	if held == nil || [4]byte(pkt[16:20]) != n.assignedV4 {
		return
	}
	copy(pkt[16:20], held[:])
	fixIPv4Header(pkt)
	fixL4(pkt, 4, n.assignedV4[:], held[:])
}

func (n *NAT) applyV6(pkt []byte, outbound bool) {
	if !n.haveV6 || len(pkt) < 40 {
		return
	}

	if outbound {
		src := [16]byte(pkt[8:24])
		if src == n.assignedV6 {
			return
		}
		if held := n.realV6.Load(); held == nil || *held != src {
			n.realV6.Store(&src)
		}
		copy(pkt[8:24], n.assignedV6[:])
		fixL4(pkt, 6, src[:], n.assignedV6[:])
		return
	}

	held := n.realV6.Load()
	if held == nil || [16]byte(pkt[24:40]) != n.assignedV6 {
		return
	}
	copy(pkt[24:40], held[:])
	fixL4(pkt, 6, n.assignedV6[:], held[:])
}

// fixIPv4Header пересчитывает контрольную сумму IPv4-заголовка.
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

// fixL4 инкрементально правит L4-контрольную сумму при замене адреса old→new.
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
	case 6: // TCP
		csumOff = l4off + 16
	case 17: // UDP
		csumOff = l4off + 6
	case 58: // ICMPv6
		csumOff = l4off + 2
	default:
		return
	}
	if csumOff+2 > len(pkt) {
		return
	}

	c := binary.BigEndian.Uint16(pkt[csumOff:])
	if proto == 17 && ver == 4 && c == 0 {
		return // UDP/IPv4 с отключённой контрольной суммой
	}
	nc := csumUpdate(c, old, new)
	if proto == 17 && nc == 0 {
		nc = 0xFFFF // в UDP 0 означает «нет суммы»
	}
	binary.BigEndian.PutUint16(pkt[csumOff:], nc)
}

// csumUpdate реализует RFC 1624: HC' = ~(~HC + ~m + m') для замены слов old→new.
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
