package windivert

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

type CaptureConfig struct {
	TCP, UDP bool
	Bypass   []netip.Prefix
	DNS      bool
}

func BuildFilter(cfg CaptureConfig) string {
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

	parts := []string{"outbound"}
	if p := protoClause(cfg); p != "" {
		parts = append(parts, p)
	}
	parts = append(parts, "("+family("ip", v4ex)+" or "+family("ipv6", v6ex)+")")
	caught := strings.Join(parts, " and ")
	if !cfg.DNS {
		return caught
	}
	return "(" + caught + ") or (outbound and ((udp and udp.DstPort == 53) or (tcp and tcp.DstPort == 53)) and " +
		"((ip and ip.DstAddr != 127.0.0.1) or (ipv6 and ipv6.DstAddr != ::1)))"
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
		return "(tcp or udp or (icmp and icmp.Type == 8))"
	case cfg.TCP:
		return "tcp"
	case cfg.UDP:
		return "udp"
	default:
		return ""
	}
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
