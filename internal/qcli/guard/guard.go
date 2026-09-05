package guard

import "net/netip"

var defaultBypass = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),      // "this host"
	netip.MustParsePrefix("10.0.0.0/8"),     // RFC1918
	netip.MustParsePrefix("100.64.0.0/10"),  // CGNAT (RFC6598)
	netip.MustParsePrefix("127.0.0.0/8"),    // loopback
	netip.MustParsePrefix("169.254.0.0/16"), // link-local
	netip.MustParsePrefix("172.16.0.0/12"),  // RFC1918
	netip.MustParsePrefix("192.168.0.0/16"), // RFC1918
	netip.MustParsePrefix("224.0.0.0/4"),    // multicast
	netip.MustParsePrefix("255.255.255.255/32"),
	netip.MustParsePrefix("::1/128"),   // loopback v6
	netip.MustParsePrefix("fc00::/7"),  // ULA
	netip.MustParsePrefix("fe80::/10"), // link-local v6
	netip.MustParsePrefix("ff00::/8"),  // multicast v6
}

type Guard struct {
	bypass  []netip.Prefix
	servers map[netip.Addr]struct{}
}

func New(serverIPs []netip.Addr) *Guard {
	g := &Guard{
		bypass:  append([]netip.Prefix(nil), defaultBypass...),
		servers: make(map[netip.Addr]struct{}, len(serverIPs)),
	}
	for _, ip := range serverIPs {
		g.servers[ip] = struct{}{}
	}
	return g
}

func (g *Guard) Bypass(dst netip.Addr) bool {
	if _, ok := g.servers[dst]; ok {
		return true
	}
	for _, p := range g.bypass {
		if p.Contains(dst) {
			return true
		}
	}
	return false
}

func (g *Guard) Bypasses() []netip.Prefix { return g.bypass }

func (g *Guard) AddServer(ip netip.Addr) { g.servers[ip] = struct{}{} }

func (g *Guard) AddBypass(p netip.Prefix) { g.bypass = append(g.bypass, p) }
