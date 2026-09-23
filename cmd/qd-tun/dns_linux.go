//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	resolvConf   = "/etc/resolv.conf"
	resolvBackup = "/run/qd-client.resolv.conf"
)

func pointDNS(link string) (func(), error) {
	if resolvedActive() {
		for _, args := range [][]string{
			{"dns", link, tunDNS},
			{"domain", link, "~."},
			{"default-route", link, "yes"},
		} {
			if out, err := run("resolvectl", args...); err != nil {
				return nil, fmt.Errorf("resolvectl %s: %s", args[0], out)
			}
		}
		fmt.Printf("dns      systemd-resolved sends every name to %s through %s\n", tunDNS, link)
		return func() { run("resolvectl", "revert", link) }, nil
	}
	return takeResolvConf()
}

func resolvedActive() bool {
	if _, err := exec.LookPath("resolvectl"); err != nil {
		return false
	}
	if target, err := filepath.EvalSymlinks(resolvConf); err == nil && strings.HasPrefix(target, "/run/systemd/resolve/") {
		return true
	}
	raw, err := os.ReadFile(resolvConf)
	return err == nil && strings.Contains(string(raw), "127.0.0.53")
}

func takeResolvConf() (func(), error) {
	head := "file:"
	if target, err := os.Readlink(resolvConf); err == nil {
		head = "link:" + target
	}
	body, _ := os.ReadFile(resolvConf)
	if err := os.WriteFile(resolvBackup, append([]byte(head+"\n"), body...), 0o600); err != nil {
		return nil, fmt.Errorf("back up %s: %w", resolvConf, err)
	}

	os.Remove(resolvConf)
	held := "# qd-client holds this file while the tunnel is up\nnameserver " + tunDNS + "\n"
	if err := os.WriteFile(resolvConf, []byte(held), 0o644); err != nil {
		restoreDNS()
		return nil, fmt.Errorf("write %s: %w", resolvConf, err)
	}
	fmt.Printf("dns      %s points at %s until the tunnel goes down\n", resolvConf, tunDNS)
	return restoreDNS, nil
}

func restoreDNS() {
	raw, err := os.ReadFile(resolvBackup)
	if err != nil {
		return
	}
	head, body, _ := strings.Cut(string(raw), "\n")

	os.Remove(resolvConf)
	if target, ok := strings.CutPrefix(head, "link:"); ok && target != "" {
		os.Symlink(target, resolvConf)
	} else {
		os.WriteFile(resolvConf, []byte(body), 0o644)
	}
	os.Remove(resolvBackup)
}
