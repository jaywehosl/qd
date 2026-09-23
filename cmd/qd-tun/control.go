//go:build windows || linux

package main

import (
	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/qwire"
)

var nodeTalk = qwire.NewKept(keepSocket)

func syncRelays(db *clientstate.DB) {
	nodeTalk.SetRelays(db.RelayLinks())
}
