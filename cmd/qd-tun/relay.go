//go:build windows || linux

package main

import (
	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/peers"
	"github.com/jaywehosl/qd/internal/relay"
)

var relays = &relay.Pool{
	Riding: func() int {
		if held := liveTunnel.Load(); held != nil {
			return (*held).RelayID()
		}
		return 0
	},
	Fresh: func(hosts []string) {
		if liveTunnel.Load() != nil {
			go keepAside(peers.Prefixes(hosts, nil))
		}
	},
}

func settingsRelays(db *clientstate.DB) {
	list, _ := db.Relays()
	settings, _ := db.Settings()
	relays.Set(list, settings.RelayMode)
}
