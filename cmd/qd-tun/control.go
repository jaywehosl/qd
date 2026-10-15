//go:build windows || linux

package main

import (
	"github.com/jaywehosl/qd/internal/qwire"
)

var nodeTalk = qwire.NewKept(keepSocket)
