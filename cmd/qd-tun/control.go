//go:build windows

package main

import (
	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/qwire"
)

var nodeTalk = qwire.New()

func syncRelays(db *clientstate.DB) {
	nodeTalk.SetRelays(db.RelayLinks())
}
