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

func (claim deviceClaim) device() store.Device {
	return store.Device{
		Fingerprint: claim.Fingerprint,
		Platform:    claim.Platform,
		Model:       claim.Model,
		Kind:        claim.Kind,
		Name:        claim.Name,
	}
}

func (state *controlState) admit(client netstate.Client, group *netstate.Group, claim deviceClaim) string {
	if claim.Fingerprint == "" {
		return ""
	}

	known, seen, err := state.db.Device(client.ID, claim.Fingerprint)
	if err != nil {
		return ""
	}
	if seen && known.Blocked {
		return "this device has been blocked by the administrator"
	}

	if !seen {
		if limit := deviceLimit(client, group); limit > 0 {
			count, err := state.db.CountDevices(client.ID)
			if err == nil && count >= limit {
				return "this subscription already uses its allowance of devices"
			}
		}
	}

	state.db.RecordDevice(client.ID, state.id, claim.device(), time.Now().UnixMilli())
	return ""
}

func deviceLimit(client netstate.Client, group *netstate.Group) int {
	if client.DeviceLimit > 0 || group == nil {
		return client.DeviceLimit
	}
	return group.DeviceLimit
}

func (state *controlState) seeAgain(client netstate.Client, claim deviceClaim) {
	if claim.Fingerprint == "" {
		return
	}
	if _, seen, err := state.db.Device(client.ID, claim.Fingerprint); err != nil || !seen {
		return
	}
	state.db.RecordDevice(client.ID, state.id, claim.device(), time.Now().UnixMilli())
}
