//go:build linux

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/jaywehosl/quic-diver/internal/clientapi"
	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/localapi"
)

const autostartFile = "/etc/xdg/autostart/qd-client-tray.desktop"

func defaultStatePath() string {
	if os.Geteuid() == 0 {
		return "/var/lib/qd-client/client.db"
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "qd-client.db"
	}
	return filepath.Join(dir, "qd-client", "client.db")
}

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func holdAutostart(on bool) error {
	if !on {
		if err := os.Remove(autostartFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("autostart: %w", err)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	entry := "[Desktop Entry]\nType=Application\nName=qd\nComment=qd tray icon\n" +
		"Exec=" + filepath.Join(filepath.Dir(exe), "qd-client-window") + " --tray\n" +
		"Icon=qd-client\nNoDisplay=true\nX-GNOME-Autostart-enabled=true\n"
	if err := os.MkdirAll(filepath.Dir(autostartFile), 0o755); err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	if err := os.WriteFile(autostartFile, []byte(entry), 0o644); err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	return nil
}

func claimInstance() (bool, func()) {
	f, err := os.OpenFile(lockPath(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		fmt.Printf("single   no lock file, running anyway: %v\n", err)
		return true, func() {}
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return false, func() {}
	}
	return true, func() {
		unix.Flock(int(f.Fd()), unix.LOCK_UN)
		f.Close()
	}
}

func lockPath() string {
	if os.Geteuid() == 0 {
		return "/run/qd-client.lock"
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "qd-client.lock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("qd-client-%d.lock", os.Getuid()))
}

func knock() bool { return false }

func answerKnocks(show func(), stop <-chan struct{}) {}

func redirectOutput(statePath string) {}

func standFull() {}

func sharpTimers() func() { return func() {} }

func runFast() {}

func startShell(db *clientstate.DB, tun *tunnel, ui *localapi.Server, api *clientapi.API, quit chan struct{}, stop <-chan struct{}) (bool, func()) {
	return false, func() {}
}

func openPage(url string) {
	fmt.Printf("page     %s\n", url)
}

const (
	guardPage = true
	headless  = true
	paneDir   = "/run/qd-client"
	paneFile  = paneDir + "/ui.json"
	paneGroup = "qd-client"
)

func bindPane(statePath, url, token string) {
	if os.Geteuid() != 0 {
		return
	}
	blob, err := json.Marshal(map[string]string{"url": url, "token": token})
	if err != nil {
		return
	}
	if err := os.MkdirAll(paneDir, 0o755); err != nil {
		fmt.Printf("window   %s: %v\n", paneDir, err)
		return
	}

	mode, gid := os.FileMode(0o600), 0
	if g, err := user.LookupGroup(paneGroup); err == nil {
		if id, err := strconv.Atoi(g.Gid); err == nil {
			mode, gid = 0o640, id
		}
	} else {
		fmt.Printf("window   no %s group, only root can open the window\n", paneGroup)
	}

	part := paneFile + ".part"
	if err := os.WriteFile(part, blob, mode); err != nil {
		fmt.Printf("window   %s: %v\n", part, err)
		return
	}
	os.Chown(part, 0, gid)
	os.Chmod(part, mode)
	if err := os.Rename(part, paneFile); err != nil {
		fmt.Printf("window   %s: %v\n", paneFile, err)
	}
}
