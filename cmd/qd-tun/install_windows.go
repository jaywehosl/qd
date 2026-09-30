//go:build windows && !core

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/update"
)

const (
	updateAsset = "qd-client-windows-amd64.exe"
	handOver    = "-update-after"
	trialRun    = 20 * time.Second
)

func init() {
	if len(os.Args) > 2 && os.Args[1] == handOver {
		watchUpdate(os.Args[2:])
		os.Exit(0)
	}
}

func (p hostPlatform) Install(tag string, open update.Opener, tick func(done, total int64)) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	fresh, err := update.Take(open, updateAsset, filepath.Join(filepath.Dir(exe), "qd-update"), tick)
	if err != nil {
		return err
	}

	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("could not move the running client aside: %w", err)
	}
	if err := os.Rename(fresh, exe); err != nil {
		os.Rename(old, exe)
		return fmt.Errorf("could not put %s in place: %w", tag, err)
	}

	args := os.Args[1:]
	if p.tun.Running() && !slices.Contains(args, "-connect") {
		args = append(args, "-connect")
	}
	watcher := exec.Command(old, append([]string{handOver, strconv.Itoa(os.Getpid())}, args...)...)
	watcher.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	if err := watcher.Start(); err != nil {
		os.Rename(exe, fresh)
		os.Rename(old, exe)
		return fmt.Errorf("could not hand over to %s: %w", tag, err)
	}
	watcher.Process.Release()

	fmt.Printf("update   %s is in place, the client restarts into it\n", tag)
	select {
	case updated <- struct{}{}:
	default:
	}
	return nil
}

func watchUpdate(args []string) {
	pid, err := strconv.Atoi(args[0])
	self, err2 := os.Executable()
	if err != nil || err2 != nil || !strings.HasSuffix(self, ".old") {
		return
	}
	exe := strings.TrimSuffix(self, ".old")
	rest := args[1:]

	if h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid)); err == nil {
		windows.WaitForSingleObject(h, 60000)
		windows.CloseHandle(h)
	}

	if start(exe, rest, trialRun) {
		return
	}
	os.Remove(exe + ".bad")
	if os.Rename(exe, exe+".bad") != nil || os.Rename(self, exe) != nil {
		return
	}
	start(exe, rest, 0)
}

func start(exe string, args []string, trial time.Duration) bool {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return false
	}
	if trial == 0 {
		cmd.Process.Release()
		return true
	}
	gone := make(chan struct{})
	go func() {
		cmd.Wait()
		close(gone)
	}()
	select {
	case <-gone:
		return false
	case <-time.After(trial):
		return true
	}
}

func settleUpdate(db *clientstate.DB) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if os.Remove(exe+".bad") == nil {
		db.Notify("error", "The update did not start, so the previous version is back.", time.Now().UnixMilli())
	}
	go func() {
		for i := 0; i < 12; i++ {
			if err := os.Remove(exe + ".old"); err == nil || os.IsNotExist(err) {
				break
			}
			time.Sleep(10 * time.Second)
		}
		os.RemoveAll(filepath.Join(filepath.Dir(exe), "qd-update"))
	}()
}
