//go:build linux

package main

import (
	"strings"
	"time"

	"github.com/jaywehosl/qd/internal/netstate"
	"github.com/jaywehosl/qd/internal/store"
	"github.com/jaywehosl/qd/internal/update"
)

type deviceClaim struct {
	Fingerprint string `json:"device"`
	Platform    string `json:"platform"`
	Model       string `json:"model"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Build       string `json:"build"`
}

func (claim deviceClaim) device() store.Device {
	return store.Device{
		Fingerprint: claim.Fingerprint,
		Platform:    claim.Platform,
		Model:       claim.Model,
		Kind:        claim.Kind,
		Name:        claim.Name,
		Version:     claim.built(),
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

func (claim deviceClaim) built() string {
	if claim.Build == "core" && claim.Version != "" {
		return claim.Version + " core"
	}
	return claim.Version
}

func judged(settings store.NetworkSettings, group *netstate.Group, claim deviceClaim) update.Verdict {
	dev, core := false, false
	if group != nil {
		dev, core = group.AllowDev, group.AllowCore
	}
	return update.Judge(claim.Version, claim.Build, settings.ClientVersion,
		strings.Split(settings.ClientReleases, "\n"), dev, core)
}
