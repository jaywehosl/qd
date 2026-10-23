//go:build linux

package main

import (
	"net/netip"
	"time"

	"github.com/jaywehosl/qd/internal/qdcrypt"
	"github.com/jaywehosl/qd/internal/store"
)

func (state *controlState) recordTelemetry() {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for range t.C {
		state.flush()
	}
}

func (state *controlState) flush() {
	stats := state.sessionStats()
	clients, err := state.data().Clients()
	if err != nil {
		return
	}

	owner := make(map[uint32]int, len(clients))
	for _, c := range clients {
		if c.UUID != "" {
			owner[qdcrypt.SessionID(c.UUID)] = c.ID
		}
	}

	now := time.Now().UnixMilli()
	readings := make([]store.Reading, 0, len(stats))
	exits := []store.Reading{}
	for _, s := range stats {
		id, known := owner[s.Session]
		if !known {
			continue
		}
		reading := store.Reading{
			ClientID: id, NodeID: state.id, Epoch: state.epoch,
			Up: s.Up, Down: s.Down, At: now,
		}
		if s.Transit {
			exits = append(exits, reading)
			continue
		}
		readings = append(readings, reading)
		if len(s.Seen) == 0 {
			continue
		}
		seen := make(map[string]int64, len(s.Seen))
		for _, a := range s.Seen {
			if !worthLogging(a.IP) {
				continue
			}
			seen[a.IP] = a.LastSeen
		}
		state.data().RecordAddresses(id, state.id, s.Device, seen)
	}

	state.data().RecordTraffic(readings)
	state.data().RecordExits(exits)
}

func worthLogging(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return !addr.IsPrivate() && !addr.IsLoopback() && !addr.IsLinkLocalUnicast() && !addr.IsUnspecified()
}
