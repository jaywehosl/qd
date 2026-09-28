//go:build windows || linux

package main

import (
	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/qwire"
)

var nodeTalk = qwire.NewKept(keepSocket)

func syncRelays(db *clientstate.DB) {
	nodeTalk.SetRelays(db.RelayLinks())
}
