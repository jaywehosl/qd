//go:build windows

package main

import (
	"fmt"
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
	return filepath.Join(dir, "qd", "client.db")
}

func moveLegacyState() {
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	old := filepath.Join(dir, "QuicDiver")
	fresh := filepath.Join(dir, "qd")
	entries, err := os.ReadDir(old)
	if err != nil {
		return
	}
	if _, err := os.Stat(filepath.Join(fresh, "client.db")); err == nil {
		return
	}
	if err := os.MkdirAll(fresh, 0o755); err != nil {
		return
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "client.db") {
			continue
		}
		if err := os.Rename(filepath.Join(old, e.Name()), filepath.Join(fresh, e.Name())); err != nil {
			fmt.Printf("state    could not move %s from %s: %v\n", e.Name(), old, err)
			return
		}
	}
	fmt.Printf("state    moved from %s to %s\n", old, fresh)
	os.RemoveAll(old)
}

const appName = "qd"
