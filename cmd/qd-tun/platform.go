//go:build windows || linux

package main

import (
	"errors"

	"github.com/jaywehosl/qd/internal/clientapi"
	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/qdcrypt"
	"github.com/jaywehosl/qd/internal/qsrv/uplink/relay"
)

type hostPlatform struct {
	tun *tunnel
	db  *clientstate.DB
}

func (p hostPlatform) Running() bool { return p.tun.Running() }

func (p hostPlatform) Failed() bool { return p.tun.Failed() }

func (p hostPlatform) Start(servers []string, relays []relay.Link, session uint32) error {
	if err := p.tun.Start(servers, relays, session); !errors.Is(err, errAlreadyUp) {
		return err
	}
	return nil
}

func (p hostPlatform) Stop() error { return p.tun.Stop() }

func (p hostPlatform) SetKey(key *qdcrypt.Key) { p.tun.SetKey(key) }

func (p hostPlatform) ServerName() string { return p.tun.ServerName() }

func (p hostPlatform) SetExit(egress bool) {
	setExit(egress)
	if d := p.tun.DNS(); d != nil {
		d.Flush()
	}
	go flushSystemDNS()
}

func (p hostPlatform) SetFixedRate(mbit int) { setFixedRate(mbit) }

func (p hostPlatform) SyncControlRelays(relays []relay.Link) { nodeTalk.SetRelays(relays) }

func (p hostPlatform) Wire() clientapi.Asker { return nodeTalk }

func (p hostPlatform) Identify() clientapi.Device { return deviceOf() }

func (p hostPlatform) HoldAutostart(on bool) error { return holdAutostart(on) }

func (p hostPlatform) Processes() []clientapi.Process {
	running := runningProcesses()
	out := make([]clientapi.Process, 0, len(running))
	for _, r := range running {
		out = append(out, clientapi.Process{
			Name: r.Name, Path: r.Path, Icon: r.Icon, PID: r.PID, Connections: r.Connections,
		})
	}
	return out
}

func (p hostPlatform) RulesChanged() { reloadProcessRules(p.db) }

func deviceOf() clientapi.Device {
	me := identify()
	return clientapi.Device{
		ID: me.ID, Platform: me.Platform, Model: me.Model, Kind: me.Kind, Name: me.Name,
	}
}

type device struct {
	ID       string `json:"device"`
	Platform string `json:"platform"`
	Model    string `json:"model"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
}
