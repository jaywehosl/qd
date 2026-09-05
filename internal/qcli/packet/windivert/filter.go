package windivert

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

// CaptureConfig — гибкое управление перехватом (arch: по семействам, протоколам и
// портам — глобально или выборочно). Строит WinDivert filter для NETWORK-слоя,
// только исходящий трафик.
//
// Per-process перехват сюда не входит: на NETWORK-слое нет PID. Он делается
// отдельным механизмом (наблюдение SOCKET-слоя: PID→5-tuple, затем сужение
// фильтра/пост-фильтрация) — следующий под-шаг B.
type CaptureConfig struct {
	// IPv4/IPv6 — какие семейства перехватывать. Оба false → оба (по умолчанию).
	IPv4, IPv6 bool
	// TCP/UDP — какие протоколы. Оба false → все протоколы.
	TCP, UDP bool
	// Ports — конкретные dst-порты; пусто → все порты.
	Ports []uint16
	// Bypass — префиксы, которые НЕ перехватывать (локалка + IP узлов). Первый
	// рубеж arch5 + анти-петля: драйвер даже не отдаёт их в userspace.
	Bypass []netip.Prefix
}

// BuildFilter собирает WinDivert filter-выражение из конфигурации.
//
// Форма: outbound and [proto] and [ports] and ( (ip and <v4-исключения>) or
// (ipv6 and <v6-исключения>) ). Разделение по семействам обязательно: в WinDivert
// поле чужого семейства делает тест ложным, а `not` применим лишь к одиночному
// тесту (не к скобке), поэтому исключения выражаются как «вне диапазона»
// (< lo or > hi) и `!= addr`, сгруппированные под `ip`/`ipv6`.
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

// family группирует условия одного семейства под тестом семейства: "(ip and ...)".
func family(fam string, clauses []string) string {
	if len(clauses) == 0 {
		return fam
	}
	return "(" + fam + " and " + strings.Join(clauses, " and ") + ")"
}

// notIn выражает «адрес не в префиксе»: != для одиночного IP, иначе вне диапазона.
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

// portClause — "(tcp.DstPort == a or udp.DstPort == a or ...)".
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

// prefixRange возвращает первый и последний адрес подсети.
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
