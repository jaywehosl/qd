package windivert

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

type CaptureConfig struct {
	IPv4, IPv6 bool
	TCP, UDP   bool
	Ports      []uint16
	Bypass     []netip.Prefix
}

func BuildFilter(cfg CaptureConfig) string {
	v4, v6 := cfg.IPv4, cfg.IPv6
	if !v4 && !v6 {
		v4, v6 = true, true
	}

	var v4ex, v6ex []string
	for _, p := range cfg.Bypass {
		if p.Addr().Is6() {
			v6ex = append(v6ex, notIn("ipv6.DstAddr", p))
		} else {
			v4ex = append(v4ex, notIn("ip.DstAddr", p))
		}
	}
	sort.Strings(v4ex)
	sort.Strings(v6ex)

	var fams []string
	if v4 {
		fams = append(fams, family("ip", v4ex))
	}
	if v6 {
		fams = append(fams, family("ipv6", v6ex))
	}
	famExpr := strings.Join(fams, " or ")
	if len(fams) > 1 {
		famExpr = "(" + famExpr + ")"
	}

	parts := []string{"outbound"}
	if p := protoClause(cfg); p != "" {
		parts = append(parts, p)
	}
	if len(cfg.Ports) > 0 {
		parts = append(parts, portClause(cfg))
	}
	parts = append(parts, famExpr)
	return strings.Join(parts, " and ")
}

func family(fam string, clauses []string) string {
	if len(clauses) == 0 {
		return fam
	}
	return "(" + fam + " and " + strings.Join(clauses, " and ") + ")"
}

func notIn(field string, p netip.Prefix) string {
	if p.IsSingleIP() {
		return fmt.Sprintf("%s != %s", field, p.Addr())
	}
	lo, hi := prefixRange(p)
	return fmt.Sprintf("(%s < %s or %s > %s)", field, lo, field, hi)
}

func protoClause(cfg CaptureConfig) string {
	switch {
	case cfg.TCP && cfg.UDP:
		return "(tcp or udp)"
	case cfg.TCP:
		return "tcp"
	case cfg.UDP:
		return "udp"
	default:
		return ""
	}
}

func portClause(cfg CaptureConfig) string {
	tcp := cfg.TCP || (!cfg.TCP && !cfg.UDP)
	udp := cfg.UDP || (!cfg.TCP && !cfg.UDP)

	var cl []string
	for _, p := range cfg.Ports {
		if tcp {
			cl = append(cl, fmt.Sprintf("tcp.DstPort == %d", p))
		}
		if udp {
			cl = append(cl, fmt.Sprintf("udp.DstPort == %d", p))
		}
	}
	return "(" + strings.Join(cl, " or ") + ")"
}

func prefixRange(p netip.Prefix) (netip.Addr, netip.Addr) {
	p = p.Masked()
	lo := p.Addr()
	bits := p.Bits()
	if lo.Is4() {
		v := lo.As4()
		host := 32 - bits
		u := binary.BigEndian.Uint32(v[:])
		if host >= 32 {
			u = 0xFFFFFFFF
		} else {
			u |= (uint32(1) << host) - 1
		}
		binary.BigEndian.PutUint32(v[:], u)
		return lo, netip.AddrFrom4(v)
	}
	v := lo.As16()
	for i := bits; i < 128; i++ {
		v[i/8] |= 1 << (7 - uint(i%8))
	}
	return lo, netip.AddrFrom16(v)
}
