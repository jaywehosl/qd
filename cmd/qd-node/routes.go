//go:build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/jaywehosl/quic-diver/internal/dnsproxy"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
	"github.com/jaywehosl/quic-diver/internal/steerlist"
)

const abroadWait = 2 * time.Second

type routeList struct {
	names    map[string]struct{}
	prefixes []netip.Prefix
}

func parseRouteList(text string) routeList {
	out := routeList{names: map[string]struct{}{}}
	for _, line := range strings.Split(text, "\n") {
		if cut := strings.IndexByte(line, '#'); cut >= 0 {
			line = line[:cut]
		}
		for _, entry := range strings.Fields(strings.ReplaceAll(line, ",", " ")) {
			entry = strings.ToLower(strings.Trim(entry, "."))
			entry = strings.TrimPrefix(entry, "*.")
			if entry == "" {
				continue
			}
			if p, err := netip.ParsePrefix(entry); err == nil {
				out.prefixes = append(out.prefixes, p.Masked())
				continue
			}
			if a, err := netip.ParseAddr(entry); err == nil {
				out.prefixes = append(out.prefixes, netip.PrefixFrom(a, a.BitLen()))
				continue
			}
			out.names[entry] = struct{}{}
		}
	}
	return out
}

func (l *routeList) matches(name string) bool {
	name = strings.TrimSuffix(name, ".")
	for name != "" {
		if _, ok := l.names[name]; ok {
			return true
		}
		dot := strings.IndexByte(name, '.')
		if dot < 0 {
			return false
		}
		name = name[dot+1:]
	}
	return false
}

func (state *controlState) loadRoutes() {
	settings, _ := state.db.NetworkSettings()
	list := parseRouteList(settings.RouteList + "\n" + steerlist.Expand(settings.RouteServices))
	was := state.routes.Swap(&list)
	state.node.SteerPrefixes(list.prefixes)
	if gone := state.node.SteerKeepOnly(list.matches); gone > 0 {
		fmt.Printf("routes     %d remembered addresses dropped, their names left the list\n", gone)
	}
	if was == nil || len(was.names) != len(list.names) || len(was.prefixes) != len(list.prefixes) {
		fmt.Printf("routes     %d names and %d networks leave through an exit for groups that route by DNS\n",
			len(list.names), len(list.prefixes))
	}
}

func (state *controlState) resolveAbroad(auth string, query []byte) ([]byte, bool) {
	list := state.routes.Load()
	if list == nil || len(list.names) == 0 || auth == "" || !state.gate.routes(qdcrypt.SessionID(auth)) {
		return nil, false
	}
	name, _, ok := dnsproxy.Question(query)
	if !ok || !list.matches(name) {
		return nil, false
	}
	return state.askAbroad(name, query)
}

func (state *controlState) askAbroad(name string, query []byte) ([]byte, bool) {
	body, err := json.Marshal(map[string]any{"query": query})
	if err != nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), abroadWait)
	defer cancel()
	raw, err := state.node.AskExit(ctx, "dns", body)
	if err != nil {
		return nil, false
	}

	var got struct {
		Answer []byte `json:"answer"`
	}
	if json.Unmarshal(raw, &got) != nil || len(got.Answer) < 12 {
		return nil, false
	}
	state.node.Steer(name, dnsproxy.Addrs(got.Answer))
	return got.Answer, true
}
