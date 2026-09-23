//go:build linux

package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func identify() device {
	vendor, product, serial := dmi("sys_vendor"), dmi("product_name"), dmi("product_serial")

	parts := []string{}
	for _, p := range []string{machineID(), vendor, product, serial} {
		if p != "" {
			parts = append(parts, p)
		}
	}

	host, _ := os.Hostname()
	if len(parts) == 0 {
		parts = append(parts, host)
	}

	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))

	model := strings.TrimSpace(vendor + " " + product)
	if model == "" {
		model = "PC"
	}

	return device{
		ID:       hex.EncodeToString(sum[:])[:32],
		Platform: platformName(),
		Model:    model,
		Kind:     chassis(),
		Name:     host,
	}
}

func machineID() string {
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if raw, err := os.ReadFile(path); err == nil {
			if id := strings.TrimSpace(string(raw)); id != "" {
				return "mid:" + id
			}
		}
	}
	return ""
}

func dmi(name string) string {
	raw, err := os.ReadFile(filepath.Join("/sys/class/dmi/id", name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func chassis() string {
	switch dmi("chassis_type") {
	case "8", "9", "10", "14", "30", "31", "32":
		return "laptop"
	}
	if found, _ := filepath.Glob("/sys/class/power_supply/BAT*"); len(found) > 0 {
		return "laptop"
	}
	return "desktop"
}

func platformName() string {
	name := "Linux"
	if f, err := os.Open("/etc/os-release"); err == nil {
		scan := bufio.NewScanner(f)
		for scan.Scan() {
			if pretty, ok := strings.CutPrefix(scan.Text(), "PRETTY_NAME="); ok {
				if pretty = strings.Trim(pretty, `"'`); pretty != "" {
					name = pretty
				}
				break
			}
		}
		f.Close()
	}

	var u unix.Utsname
	if unix.Uname(&u) == nil {
		if release := unix.ByteSliceToString(u.Release[:]); release != "" {
			return name + " (kernel " + release + ")"
		}
	}
	return name
}
