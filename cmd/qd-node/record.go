//go:build linux

package main

import (
	"net/netip"
	"time"

	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
	"github.com/jaywehosl/quic-diver/internal/store"
)

func (state *controlState) recordTelemetry() {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for range t.C {
		state.flush()
	}
}

func (state *controlState) flush() {
	if state.sessions == nil || state.sessions.stat == nil {
		return
	}
	stats, err := state.sessions.stat()
	if err != nil {
		return
	}
	clients, err := state.db.Clients()
	if err != nil {
		return
	}

	owner := make(map[uint32]int, len(clients))
	for _, c := range clients {
		if c.UUID != "" {
			owner[qdcrypt.SessionID(c.UUID)] = c.ID
		}
	}

	carrier := map[uint32]int{}
	if nodes, err := state.db.Nodes(); err == nil {
		for _, n := range nodes {
			if n.ID != state.id && n.UUID != "" {
				carrier[qdcrypt.SessionID(n.UUID)] = n.ID
			}
		}
	}

	now := time.Now().UnixMilli()
	readings := make([]store.Reading, 0, len(stats))
	peers := make([]store.Reading, 0, len(carrier))
	for _, s := range stats {
		id, known := owner[s.Session]
		if !known {
			if peer, carried := carrier[s.Session]; carried {
				peers = append(peers, store.Reading{
					ClientID: peer, NodeID: state.id, Epoch: state.epoch,
					Up: s.Up, Down: s.Down, At: now,
				})
			}
			continue
		}
		readings = append(readings, store.Reading{
			ClientID: id, NodeID: state.id, Epoch: state.epoch,
			Up: s.Up, Down: s.Down, At: now,
		})

		if s.Transit {
			state.db.RecordExit(id, state.id, s.LastSeen, s.Up, s.Down)
			continue
		}
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
		state.db.RecordAddresses(id, state.id, s.Device, seen)
	}

	state.db.RecordTraffic(readings)
	state.db.RecordPeerTraffic(peers)
}

func worthLogging(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return !addr.IsPrivate() && !addr.IsLoopback() && !addr.IsLinkLocalUnicast() && !addr.IsUnspecified()
}
