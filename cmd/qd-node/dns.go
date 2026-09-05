//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/jaywehosl/quic-diver/internal/dnsproxy"
	"github.com/jaywehosl/quic-diver/internal/store"
)

func (state *controlState) dnsConfig() dnsproxy.Config {
	settings, err := state.db.NetworkSettings()
	if err != nil {
		settings = store.NetworkSettings{
			DNSPrimary: "1.1.1.1", DNSSecondary: "8.8.8.8",
			DNSCache: 4096, DNSMinTTL: 10, DNSMaxTTL: 3600, DNSStale: 60,
		}
	}

	rows, err := state.db.DNSRecords()
	if err != nil {
		rows = nil
	}
	records := make([]dnsproxy.Record, 0, len(rows))
	for _, r := range rows {
		if !r.Enable {
			continue
		}
		records = append(records, dnsproxy.Record{Suffix: r.Suffix, V4: r.V4, V6: r.V6})
	}

	primary, secondary := settings.DNSPrimary, settings.DNSSecondary
	if me, found := state.selfRow(); found && (me.DNSPrimary != "" || me.DNSSecondary != "") {
		primary, secondary = me.DNSPrimary, me.DNSSecondary
	}
	primary, secondary = reachableUpstreams(primary, secondary)

	return dnsproxy.Config{
		Upstreams: dnsproxy.Addresses(primary, secondary),
		Records:   records,
		Cache:     settings.DNSCache,
		MinTTL:    time.Duration(settings.DNSMinTTL) * time.Second,
		MaxTTL:    time.Duration(settings.DNSMaxTTL) * time.Second,
		Stale:     time.Duration(settings.DNSStale) * time.Second,
	}
}

func (state *controlState) startResolver() {
	cfg := state.dnsConfig()
	state.dns = dnsproxy.New(cfg)
	fmt.Printf("resolver   %v, %d cached names, %d local records\n",
		cfg.Upstreams, cfg.Cache, len(cfg.Records))
}

func (state *controlState) reloadResolver() {
	if state.dns == nil {
		return
	}
	state.dns.Reconfigure(state.dnsConfig())
}

func (state *controlState) resolve(req request) response {
	if state.dns == nil {
		return response{OK: false, Error: "this node runs no resolver"}
	}

	var body struct {
		Query []byte `json:"query"`
	}
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return response{OK: false, Error: err.Error()}
	}
	if len(body.Query) < 12 {
		return response{OK: false, Error: "dns: query too short"}
	}

	answer, hit, err := state.dns.Answer(body.Query)
	if err != nil {
		return response{OK: false, Error: err.Error()}
	}
	answer = state.standIn(body.Query, answer)
	answer = state.dropUnreachable(body.Query, answer)
	return reply(req, map[string]any{"answer": answer, "hit": hit})
}

func (state *controlState) upstreamsSaid() string {
	me, found := state.selfRow()
	if !found || (me.DNSPrimary == "" && me.DNSSecondary == "") {
		return "the network settings"
	}
	if me.DNSSecondary == "" {
		return me.DNSPrimary
	}
	return me.DNSPrimary + " and " + me.DNSSecondary
}

// standIn достраивает ответ для имён, живущих только в IPv6. Клиент говорит по
// IPv4 и такое имя открыть не может: спрашивает A, получает пустой ответ и
// сдаётся. Узел спрашивает AAAA сам, выдаёт имени подставной IPv4 и запоминает
// пару — а при дозвоне разворачивает её обратно в настоящий адрес.
func (state *controlState) standIn(query, answer []byte) []byte {
	if state.node == nil || len(answer) < 12 || !hasGlobalV6() {
		return answer
	}
	_, qtype, ok := dnsproxy.Question(query)
	if !ok || qtype != 1 {
		return answer
	}
	if _, found := dnsproxy.FirstAddr(answer, 1); found {
		return answer
	}

	ask := dnsproxy.AskFor(query, 28)
	if ask == nil {
		return answer
	}
	got, _, err := state.dns.Answer(ask)
	if err != nil {
		return answer
	}
	v6, found := dnsproxy.FirstAddr(got, 28)
	if !found {
		return answer
	}
	addr, ok := netip.AddrFromSlice(v6)
	if !ok {
		return answer
	}
	stand, ok := state.node.Stand(addr)
	if !ok {
		return answer
	}
	return dnsproxy.Answer(query, stand.AsSlice(), 1)
}

var v6Once sync.Once
var v6Here bool

func hasGlobalV6() bool {
	v6Once.Do(func() { v6Here = lookForV6() })
	return v6Here
}

func lookForV6() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		prefix, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		addr, ok := netip.AddrFromSlice(prefix.IP)
		if !ok || !addr.Is6() || addr.Is4In6() {
			continue
		}
		if addr.IsGlobalUnicast() && !addr.IsPrivate() && !addr.IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}

func reachableUpstreams(primary, secondary string) (string, string) {
	if hasGlobalV6() {
		return primary, secondary
	}

	kept := []string{}
	for _, one := range []string{primary, secondary} {
		if one == "" {
			continue
		}
		if addr, err := netip.ParseAddr(strings.TrimSpace(one)); err == nil && addr.Is6() {
			continue
		}
		kept = append(kept, one)
	}

	switch len(kept) {
	case 0:
		return "1.1.1.1", "8.8.8.8"
	case 1:
		return kept[0], ""
	default:
		return kept[0], kept[1]
	}
}

func (state *controlState) dropUnreachable(query, answer []byte) []byte {
	if hasGlobalV6() {
		return answer
	}
	_, qtype, ok := dnsproxy.Question(query)
	if !ok || qtype != 28 {
		return answer
	}
	if empty := dnsproxy.Empty(query); empty != nil {
		return empty
	}
	return answer
}
