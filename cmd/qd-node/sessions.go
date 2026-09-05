//go:build linux

package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/jaywehosl/quic-diver/internal/netstate"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
)

func (state *controlState) syncSessions() {
	if state.sessions == nil {
		return
	}

	network, err := state.db.LoadState()
	if err != nil {
		return
	}
	now := time.Now().UnixMilli()
	want := map[uint32]bool{}

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
		}
		for _, p := range mine.Peers {
			if p.Role == netstate.RoleIngress && p.Session != 0 {
				want[p.Session] = false
			}
		}
	}

	live, err := state.sessions.list()
	if err != nil {
		return
	}

	added, removed := 0, 0
	for id, allowExit := range want {
		if _, carried := live[id]; !carried {
			if err := state.sessions.add(id); err != nil {
				continue
			}
			added++
		}
		if state.sessions.exit != nil {
			state.sessions.exit(id, allowExit)
		}
	}
	for id := range live {
		if _, carried := want[id]; carried {
			continue
		}
		if err := state.sessions.del(id); err == nil {
			removed++
		}
	}

	exits := 0
	for _, allowExit := range want {
		if allowExit {
			exits++
		}
	}
	if added > 0 || removed > 0 || exits != state.exits {
		peers := 0
		for _, p := range mine.Peers {
			if p.Role == netstate.RoleIngress && p.Session != 0 {
				peers++
			}
		}
		fmt.Printf("sessions   %d carried (+%d, -%d), %d may take an exit, %d are peer nodes\n",
			len(want), added, removed, exits, peers)
	}
	state.exits = exits
}

type sessionMap struct {
	add   func(uint32) error
	del   func(uint32) error
	exit  func(uint32, bool) error
	list  func() (map[uint32]struct{}, error)
	stat  func() ([]sessionStat, error)
	reset func(uint32) error
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
