//go:build (windows || linux) && core

package main

import (
	"errors"

	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/update"
)

func (p hostPlatform) Install(string, update.Opener, func(done, total int64)) error {
	return errors.New("the core inside umiray is updated by umiray")
}

func settleUpdate(*clientstate.DB) {}
