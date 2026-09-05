//go:build linux

package main

import (
	"time"

	"github.com/jaywehosl/quic-diver/internal/netstate"
	"github.com/jaywehosl/quic-diver/internal/store"
)

type deviceClaim struct {
	Fingerprint string `json:"device"`
	Platform    string `json:"platform"`
	Model       string `json:"model"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
}

type verdict struct {
	Allowed bool
	Reason  string
}

func (state *controlState) admit(client netstate.Client, claim deviceClaim) verdict {
	if claim.Fingerprint == "" {
		return verdict{Allowed: true}
	}

	known, seen, err := state.db.Device(client.ID, claim.Fingerprint)
	if err != nil {
		return verdict{Allowed: true}
	}
	if seen && known.Blocked {
		return verdict{Reason: "this device has been blocked by the administrator"}
	}

	if !seen {
		if limit := state.deviceLimit(client); limit > 0 {
			count, err := state.db.CountDevices(client.ID)
			if err == nil && count >= limit {
				return verdict{Reason: "this subscription already uses its allowance of devices"}
			}
		}
	}

	state.db.RecordDevice(client.ID, state.id, store.Device{
		Fingerprint: claim.Fingerprint,
		Platform:    claim.Platform,
		Model:       claim.Model,
		Kind:        claim.Kind,
		Name:        claim.Name,
	}, time.Now().UnixMilli())

	return verdict{Allowed: true}
}

func (state *controlState) deviceLimit(client netstate.Client) int {
	if client.DeviceLimit > 0 {
		return client.DeviceLimit
	}
	if client.GroupID == 0 {
		return 0
	}

	groups, err := state.db.Groups()
	if err != nil {
		return 0
	}
	for _, g := range groups {
		if g.ID == client.GroupID {
			return g.DeviceLimit
		}
	}
	return 0
}

func (state *controlState) clientByKey(token string) (netstate.Client, bool) {
	if token == "" {
		return netstate.Client{}, false
	}
	clients, err := state.db.Clients()
	if err != nil {
		return netstate.Client{}, false
	}
	for _, c := range clients {
		if c.UUID == token {
			return c, true
		}
	}
	return netstate.Client{}, false
}

func (state *controlState) mayExit(client netstate.Client) bool {
	if client.AllowExit != netstate.ExitInherit {
		return client.MayExit(nil)
	}
	groups, err := state.db.Groups()
	if err != nil {
		return false
	}
	for i := range groups {
		if groups[i].ID == client.GroupID {
			return client.MayExit(&groups[i])
		}
	}
	return client.MayExit(nil)
}

func (state *controlState) seeAgain(client netstate.Client, claim deviceClaim) {
	if claim.Fingerprint == "" {
		return
	}
	if _, seen, err := state.db.Device(client.ID, claim.Fingerprint); err != nil || !seen {
		return
	}
	state.db.RecordDevice(client.ID, state.id, store.Device{
		Fingerprint: claim.Fingerprint,
		Platform:    claim.Platform,
		Model:       claim.Model,
		Kind:        claim.Kind,
		Name:        claim.Name,
	}, time.Now().UnixMilli())
}
