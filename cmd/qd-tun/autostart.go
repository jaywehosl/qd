//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"
)

const autostartTask = appName

func autostartHeld() bool {
	out, err := run("schtasks", "/query", "/tn", autostartTask)
	if err != nil {
		return false
	}
	return strings.Contains(out, autostartTask)
}

func holdAutostart(on bool) error {
	if !on {
		out, err := run("schtasks", "/delete", "/tn", autostartTask, "/f")
		if err != nil && autostartHeld() {
			return fmt.Errorf("autostart: %s", strings.TrimSpace(out))
		}
		return nil
	}

	binary, err := os.Executable()
	if err != nil {
		return err
	}

	out, err := run("schtasks", "/create", "/tn", autostartTask,
		"/tr", fmt.Sprintf(`"%s" -autostart`, binary),
		"/sc", "onlogon", "/rl", "highest", "/f")
	if err != nil {
		return fmt.Errorf("autostart: %s", strings.TrimSpace(out))
	}
	return nil
}
