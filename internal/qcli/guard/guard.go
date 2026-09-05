// Package guard — local-guard: решает, какой трафик НЕ захватывать.
//
// Критическое требование arch5: клиент не должен заворачивать домашний/локальный
// трафик (доступ к веб-морде роутера 192.168.x.1 и т.п. обязан работать). Плюс
// анти-петля: трафик к IP самих узлов-серверов НИКОГДА не заворачивается, иначе
// туннель зациклит сам себя.
//
// Это второй рубеж; первый — filter-выражение WinDivert/маршруты TUN, которые в
// идеале не отдают такие пакеты в захват вовсе. Guard страхует в коде.
package guard

import "net/netip"

// defaultBypass — сети, которые по умолчанию идут мимо туннеля.
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

// Guard хранит исключения захвата.
type Guard struct {
	bypass  []netip.Prefix
	servers map[netip.Addr]struct{} // IP узлов — анти-петля
}

// New строит guard с дефолтными локальными исключениями и списком IP узлов,
// к которым идёт туннель (их трафик не заворачивается — анти-петля).
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

// Bypass возвращает true, если пакет на dst НЕ надо захватывать (пустить мимо).
func (g *Guard) Bypass(dst netip.Addr) bool {
	if _, ok := g.servers[dst]; ok {
		return true // анти-петля: трафик к самому узлу
	}
	for _, p := range g.bypass {
		if p.Contains(dst) {
			return true
		}
	}
	return false
}

// Bypasses возвращает список исключаемых префиксов (для построения WinDivert
// filter — первый рубеж, чтобы драйвер не отдавал локалку в userspace).
func (g *Guard) Bypasses() []netip.Prefix { return g.bypass }

// AddServer добавляет IP узла в анти-петлю (напр. после ре-резолва домена).
func (g *Guard) AddServer(ip netip.Addr) { g.servers[ip] = struct{}{} }

// AddBypass добавляет пользовательское исключение (напр. корпоративную подсеть).
func (g *Guard) AddBypass(p netip.Prefix) { g.bypass = append(g.bypass, p) }
