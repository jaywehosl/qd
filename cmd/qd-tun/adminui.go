//go:build (windows || linux) && !core

package main

import (
	"runtime"

	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/panel"
	"github.com/jaywehosl/qd/internal/qdcrypt"
)

type adminUI = panel.Seat

func newAdminUI(key *qdcrypt.Key, db *clientstate.DB) *adminUI {
	return panel.NewSeat(key, db, nodeTalk)
}

const buildKind = runtime.GOOS
