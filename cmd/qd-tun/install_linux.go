//go:build linux && !core

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/update"
)

const updateAsset = "qd-client-linux-amd64.tar.gz"

func (p hostPlatform) Install(tag string, open update.Opener, tick func(done, total int64)) error {
	dir := filepath.Join(stateDir, "update")
	pack, err := update.Take(open, tag, updateAsset, dir, tick)
	if err != nil {
		return err
	}
	stage := filepath.Join(dir, "stage")
	os.RemoveAll(stage)
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return err
	}
	if out, err := exec.Command("tar", "-xzf", pack, "-C", stage).CombinedOutput(); err != nil {
		return fmt.Errorf("unpack %s: %v: %s", tag, err, out)
	}

	if p.tun.Running() {
		p.db.SetValue(resumeKey, "1")
	}

	unit := "qd-client-update-" + strconv.FormatInt(time.Now().Unix(), 10)
	script := filepath.Join(stage, "qd-client", "install.sh")
	if out, err := exec.Command("systemd-run", "--no-block", "--collect", "--unit="+unit, "/bin/bash", script).CombinedOutput(); err != nil {
		return fmt.Errorf("start the installer of %s: %v: %s", tag, err, out)
	}
	fmt.Printf("update   %s unpacked, its installer runs as %s and restarts the service\n", tag, unit)
	return nil
}

func settleUpdate(*clientstate.DB) {}

func startGuard(*tunnel, <-chan struct{}) {}

func tellUp() {}
