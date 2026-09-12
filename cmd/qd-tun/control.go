//go:build windows

package main

import (
	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/qwire"
)

var nodeTalk = qwire.New()

func syncRelays(db *clientstate.DB) {
	relays := db.RelayLinks()
	wr := make([]qwire.RelayLink, 0, len(relays))
	for _, r := range relays {
		wr = append(wr, qwire.RelayLink{Weblink: r.Weblink, Authority: r.Authority})
	}
	nodeTalk.SetRelays(wr)
}
