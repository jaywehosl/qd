//go:build windows

package main

import (
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
)

type stats struct {
	procMiss atomic.Uint64
}

const appName = "qd"

var st stats

var routeByProcess atomic.Pointer[procRouter]

func run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
