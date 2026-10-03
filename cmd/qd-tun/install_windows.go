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
	upEvent     = `Global\qdClientUp`
	upWait      = 30000
	leaveWait   = 20000
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
	fresh, err := update.Take(open, tag, updateAsset, filepath.Join(filepath.Dir(exe), "qd-update"), tick)
	if err != nil {
		return err
	}

	old := exe + ".old"
	clearAside(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("could not move the running client aside: %w", err)
	}
	if err := os.Rename(fresh, exe); err != nil {
		os.Rename(old, exe)
		return fmt.Errorf("could not put %s in place: %w", tag, err)
	}
	os.Remove(filepath.Dir(fresh))

	args := os.Args[1:]
	if p.tun.Running() && !slices.Contains(args, "-connect") {
		args = append(args, "-connect")
	}
	watcher := exec.Command(old, append([]string{handOver, strconv.Itoa(os.Getpid())}, args...)...)
	watcher.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	bequeath(watcher)
	if err := watcher.Start(); err != nil {
		os.Remove(exe)
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

	awaitExit(uint32(pid))

	up := namedEvent(upEvent, true)
	if up != 0 {
		windows.ResetEvent(up)
	}
	if start(exe, rest, up) {
		return
	}
	os.Remove(exe + ".bad")
	if os.Rename(exe, exe+".bad") != nil || os.Rename(self, exe) != nil {
		return
	}
	start(exe, rest, 0)
}

func start(exe string, args []string, up windows.Handle) bool {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return false
	}
	pid := uint32(cmd.Process.Pid)
	cmd.Process.Release()
	if up == 0 {
		return true
	}
	fresh, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return true
	}
	defer windows.CloseHandle(fresh)

	which, _ := windows.WaitForMultipleObjects([]windows.Handle{up, fresh}, false, upWait)
	switch which {
	case windows.WAIT_OBJECT_0:
		return true
	case windows.WAIT_OBJECT_0 + 1:
		var code uint32
		windows.GetExitCodeProcess(fresh, &code)
		return code == 0
	}
	windows.TerminateProcess(fresh, 1)
	windows.WaitForSingleObject(fresh, 5000)
	return false
}

func tellUp() {
	name, err := windows.UTF16PtrFromString(upEvent)
	if err != nil {
		return
	}
	handle, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return
	}
	windows.SetEvent(handle)
	windows.CloseHandle(handle)
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
		if stale, err := filepath.Glob(exe + ".*.gone"); err == nil {
			for _, path := range stale {
				os.Remove(path)
			}
		}
	}()
}

func clearAside(old string) {
	if err := os.Remove(old); err != nil && !os.IsNotExist(err) {
		os.Rename(old, fmt.Sprintf("%s.%d.gone", strings.TrimSuffix(old, ".old"), os.Getpid()))
	}
}

func bequeath(cmd *exec.Cmd) {
	cmd.Env = append(os.Environ(), tokenEnv+"="+paneToken)
}
