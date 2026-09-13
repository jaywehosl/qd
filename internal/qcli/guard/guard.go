package guard

import "net/netip"

var defaultBypass = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("255.255.255.255/32"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
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
