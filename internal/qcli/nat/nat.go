package nat

import (
	"encoding/binary"
	"net/netip"
	"sync/atomic"

	"github.com/jaywehosl/qd/internal/ippkt"
)

type NAT struct {
	assignedV4 [4]byte
	haveV4     bool
	assignedV6 [16]byte
	haveV6     bool

	realV4 atomic.Pointer[[4]byte]
	realV6 atomic.Pointer[[16]byte]

	portV4 [1 << 16]atomic.Uint32
	portV6 [1 << 16]atomic.Pointer[[16]byte]
}

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

	if outbound {
		src := [4]byte(pkt[12:16])
		if port, ok := udpPort(pkt, 4, 0); ok {
			if from := binary.BigEndian.Uint32(src[:]); n.portV4[port].Load() != from {
				n.portV4[port].Store(from)
			}
		}
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

	if [4]byte(pkt[16:20]) != n.assignedV4 {
		return
	}
	var real [4]byte
	from := uint32(0)
	if port, ok := n.portOfV4(pkt); ok {
		from = n.portV4[port].Load()
	}
	if from != 0 {
		binary.BigEndian.PutUint32(real[:], from)
	} else if held := n.realV4.Load(); held != nil {
		real = *held
	} else {
		return
	}
	if real == n.assignedV4 {
		return
	}
	copy(pkt[16:20], real[:])
	fixIPv4Header(pkt)
	fixL4(pkt, 4, n.assignedV4[:], real[:])
	if pkt[9] == 1 {
		n.fixQuoteV4(pkt, real)
	}
}

func (n *NAT) portOfV4(pkt []byte) (uint16, bool) {
	if pkt[9] != 1 {
		return udpPort(pkt, 4, 2)
	}
	msg := pkt[int(pkt[0]&0x0F)*4:]
	if len(msg) < 28+8 || (msg[0] != 3 && msg[0] != 11 && msg[0] != 12) {
		return 0, false
	}
	return udpPort(msg[8:], 4, 0)
}

func udpPort(pkt []byte, ver int, at int) (uint16, bool) {
	head := 40
	proto := byte(0)
	if ver == 4 {
		if len(pkt) < 20 || binary.BigEndian.Uint16(pkt[6:8])&0x1FFF != 0 {
			return 0, false
		}
		head, proto = int(pkt[0]&0x0F)*4, pkt[9]
	} else if len(pkt) >= 40 {
		proto = pkt[6]
	}
	if proto != 17 || head < 20 || len(pkt) < head+4 {
		return 0, false
	}
	return binary.BigEndian.Uint16(pkt[head+at:]), true
}

func (n *NAT) fixQuoteV4(pkt []byte, real [4]byte) {
	msg := pkt[int(pkt[0]&0x0F)*4:]
	if len(msg) < 28 || (msg[0] != 3 && msg[0] != 11 && msg[0] != 12) {
		return
	}
	inner := msg[8:]
	if [4]byte(inner[12:16]) != n.assignedV4 {
		return
	}
	copy(inner[12:16], real[:])
	fixIPv4Header(inner)
	msg[2], msg[3] = 0, 0
	binary.BigEndian.PutUint16(msg[2:], ippkt.Checksum(msg))
}

func (n *NAT) applyV6(pkt []byte, outbound bool) {
	if !n.haveV6 || len(pkt) < 40 {
		return
	}

	if outbound {
		src := [16]byte(pkt[8:24])
		if port, ok := udpPort(pkt, 6, 0); ok {
			if held := n.portV6[port].Load(); held == nil || *held != src {
				n.portV6[port].Store(&src)
			}
		}
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

	if [16]byte(pkt[24:40]) != n.assignedV6 {
		return
	}
	held := n.realV6.Load()
	if port, ok := udpPort(pkt, 6, 2); ok {
		if from := n.portV6[port].Load(); from != nil {
			held = from
		}
	}
	if held == nil || *held == n.assignedV6 {
		return
	}
	copy(pkt[24:40], held[:])
	fixL4(pkt, 6, n.assignedV6[:], held[:])
	if pkt[6] == 58 {
		n.fixQuoteV6(pkt, *held)
	}
}

func (n *NAT) fixQuoteV6(pkt []byte, real [16]byte) {
	msg := pkt[40:]
	if len(msg) < 48 || (msg[0] != 1 && msg[0] != 3) {
		return
	}
	inner := msg[8:]
	if [16]byte(inner[8:24]) != n.assignedV6 {
		return
	}
	copy(inner[8:24], real[:])
	msg[2], msg[3] = 0, 0
	binary.BigEndian.PutUint16(msg[2:], ippkt.Checksum6(pkt))
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
		if binary.BigEndian.Uint16(pkt[6:8])&0x1FFF != 0 {
			return
		}
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
