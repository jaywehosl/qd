//go:build linux

package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jaywehosl/qd/internal/netstate"
	"github.com/jaywehosl/qd/internal/qdcrypt"
)

func (state *controlState) syncSessions() {
	state.followUpdates()
	network, err := state.db.LoadState()
	if err != nil {
		return
	}
	now := time.Now().UnixMilli()
	want := map[uint32]bool{}
	routed := map[uint32]bool{}
	builds := map[uint32][2]bool{}
	linked := map[uint32]bool{}
	peers, exits, byDNS := 0, 0, 0

	mine, err := netstate.Project(state.id, network)
	switch {
	case errors.Is(err, netstate.ErrNodeOff):
	case err != nil:
		return
	default:
		for _, c := range mine.Clients {
			if c.ExpiryAt > 0 && c.ExpiryAt < now {
				continue
			}
			want[qdcrypt.SessionID(c.UUID)] = c.AllowExit
			routed[qdcrypt.SessionID(c.UUID)] = c.RouteDNS
			builds[qdcrypt.SessionID(c.UUID)] = [2]bool{c.AllowDev, c.AllowCore}
			if c.AllowExit {
				exits++
			}
			if c.RouteDNS {
				byDNS++
			}
		}
		for _, p := range mine.Peers {
			if p.Role == netstate.RoleIngress && p.Session != 0 {
				want[p.Session] = false
				linked[p.Session] = true
				peers++
			}
		}
	}

	state.gate.nodes(linked)
	live := state.gate.list()
	added, removed := 0, 0
	for id, allowExit := range want {
		if _, carried := live[id]; !carried {
			state.gate.add(id)
			added++
		}
		state.gate.exit(id, allowExit)
		state.gate.route(id, routed[id])
		state.gate.builds(id, builds[id][0], builds[id][1])
	}
	for id := range live {
		if _, carried := want[id]; carried {
			continue
		}
		state.gate.del(id)
		state.node.Forget(id)
		removed++
	}

	if added > 0 || removed > 0 || exits != state.exits || byDNS != state.byDNS {
		fmt.Printf("sessions   %d carried (+%d, -%d), %d may take an exit, %d route by DNS, %d are peer nodes\n",
			len(want), added, removed, exits, byDNS, peers)
	}
	state.exits = exits
	state.byDNS = byDNS

	alive := map[uint32]bool{}
	for _, s := range state.node.Sessions() {
		alive[s.Seat] = true
	}
	late := state.gate.outdated(alive)
	for _, seat := range late {
		state.node.Forget(seat)
	}
	if len(late) > 0 {
		fmt.Printf("update     %d tunnels of outdated clients closed\n", len(late))
	}
}

func (state *controlState) sessionStats() []sessionStat {
	live := sampleSessions(state.node)
	out := make([]sessionStat, 0, len(live))
	for id, s := range live {
		since, lastSeen, checked, fingerprint, addresses := state.watch.of(id)
		out = append(out, sessionStat{
			Session: id, Client: s.Client, Transit: s.Transit, LastSeen: lastSeen,
			Since: since, Checked: checked, Device: fingerprint, Seen: addresses,
			Up: s.Up, Down: s.Down, PktUp: s.PktUp, PktDown: s.PktDown,
		})
	}
	return out
}

type sessionStat struct {
	Session  uint32    `json:"session"`
	Client   string    `json:"client"`
	Transit  bool      `json:"transit"`
	LastSeen int64     `json:"lastSeen"`
	Since    int64     `json:"since"`
	Checked  int64     `json:"checked"`
	Device   string    `json:"device"`
	Up       uint64    `json:"up"`
	Down     uint64    `json:"down"`
	PktUp    uint64    `json:"pktUp"`
	PktDown  uint64    `json:"pktDown"`
	Seen     []address `json:"seen"`
}

func (state *controlState) followUpdates() {
	settings, err := state.db.NetworkSettings()
	if err != nil {
		return
	}
	var releases []string
	if settings.ClientReleases != "" {
		releases = strings.Split(settings.ClientReleases, "\n")
	}
	if !state.gate.policy(settings.ClientVersion, releases) {
		return
	}
	if settings.ClientVersion == "" {
		fmt.Printf("update     clients are held to no version\n")
		return
	}
	fmt.Printf("update     clients are held to %s, %d releases known\n", settings.ClientVersion, len(releases))
	go state.shelf.Stock(settings.ClientVersion)
}
