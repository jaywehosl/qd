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

const sessionSweep = 30 * time.Second

func (state *controlState) keepSessions() {
	tick := time.NewTicker(sessionSweep)
	defer tick.Stop()
	for range tick.C {
		state.syncSessions()
	}
}

func (state *controlState) syncSessions() {
	state.syncing.Lock()
	defer state.syncing.Unlock()

	state.followUpdates()
	network, err := state.data().LoadState()
	if err != nil {
		return
	}
	now := time.Now().UnixMilli()
	want := map[uint32]bool{}
	builds := map[uint32][2]bool{}
	owners := map[uint32]owner{}
	linked := map[uint32]bool{}
	peers, exits := 0, 0

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
			id := qdcrypt.SessionID(c.UUID)
			want[id] = c.AllowExit
			builds[id] = [2]bool{c.AllowDev, c.AllowCore}
			owners[id] = owner{client: c.ID, limit: c.DeviceLimit}
			if c.AllowExit {
				exits++
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
	state.gate.claim(owners)
	live := state.gate.list()
	added, removed := 0, 0
	for id, allowExit := range want {
		if _, carried := live[id]; !carried {
			state.gate.add(id)
			added++
		}
		state.gate.exit(id, allowExit)
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

	if added > 0 || removed > 0 || exits != state.exits {
		fmt.Printf("sessions   %d carried (+%d, -%d), %d may take an exit, %d are peer nodes\n",
			len(want), added, removed, exits, peers)
	}
	state.exits = exits

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
	totals := state.node.Totals()
	out := make([]sessionStat, 0, len(totals))
	for _, t := range totals {
		since, lastSeen, checked, fingerprint, addresses := state.watch.of(t.Session)
		stat := sessionStat{
			Session: t.Session, Client: live[t.Session].Client, Transit: t.Transit, LastSeen: lastSeen,
			Since: since, Checked: checked, Device: fingerprint,
			Up: t.Up, Down: t.Down, PktUp: t.PktUp, PktDown: t.PktDown,
		}
		if !t.Transit {
			stat.Seen = addresses
		}
		out = append(out, stat)
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
	settings, err := state.data().NetworkSettings()
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
