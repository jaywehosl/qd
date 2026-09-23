//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

var iphlpapi = syscall.NewLazyDLL("iphlpapi.dll")

const (
	afUnspec = 0
	afInet   = 2
	afInet6  = 23
)

func run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func defaultStatePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "qd-client.db"
	}
	return filepath.Join(dir, "QuicDiver", "client.db")
}

const appName = "qd"
