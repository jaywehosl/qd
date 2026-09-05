//go:build windows

package main

import (
	"github.com/jaywehosl/quic-diver/internal/clientapi"
	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
)

type winPlatform struct {
	tun *tunnel
	db  *clientstate.DB
}

func (p winPlatform) Running() bool { return p.tun.Running() }

func (p winPlatform) Start(servers []string, session uint32) error {
	return p.tun.Start(servers, session)
}

func (p winPlatform) Stop() error { return p.tun.Stop() }

func (p winPlatform) SetKey(key *qdcrypt.Key) { p.tun.SetKey(key) }

func (p winPlatform) ServerName() string { return p.tun.ServerName() }

func (p winPlatform) SetExit(egress bool) { setExit(egress) }

func (p winPlatform) SetFixedRate(mbit int) { setFixedRate(mbit) }

func (p winPlatform) Wire() clientapi.Asker { return nodeTalk }

func (p winPlatform) Identify() clientapi.Device { return deviceOf() }

func (p winPlatform) HoldAutostart(on bool) error { return holdAutostart(on) }

func (p winPlatform) AutostartHeld() bool { return autostartHeld() }

func (p winPlatform) Processes() []clientapi.Process {
	running := runningProcesses()
	out := make([]clientapi.Process, 0, len(running))
	for _, r := range running {
		out = append(out, clientapi.Process{
			Name: r.Name, Path: r.Path, Icon: r.Icon, PID: r.PID, Connections: r.Connections,
		})
	}
	return out
}

func (p winPlatform) RulesChanged() { reloadProcessRules(p.db) }

func deviceOf() clientapi.Device {
	me := identify()
	return clientapi.Device{
		ID: me.ID, Platform: me.Platform, Model: me.Model, Kind: me.Kind, Name: me.Name,
	}
}
