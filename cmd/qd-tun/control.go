//go:build windows || linux

package main

import (
	"github.com/jaywehosl/qd/internal/qwire"
)

var nodeTalk = func() *qwire.Dialer {
	d := qwire.NewKept(keepSocket)
	d.Relays = relays.Vias
	return d
}()
