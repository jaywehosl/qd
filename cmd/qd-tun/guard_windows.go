//go:build windows && !core

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	guardFlag  = "-guard"
	aliveEvent = `Global\qdClientAlive`
	carryEvent = `Global\qdClientCarrying`
	guardWait  = 15000
	beatEvery  = 2 * time.Second
)

func init() {
	settleCursor()
	if len(os.Args) > 2 && os.Args[1] == guardFlag {
		standGuard(os.Args[2:])
		os.Exit(0)
	}
}

var (
	peekMessage       = user32.NewProc("PeekMessageW")
	getMessage        = user32.NewProc("GetMessageW")
	postThreadMessage = user32.NewProc("PostThreadMessageW")
)

func settleCursor() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var msg [48]byte
	peekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0)
	postThreadMessage.Call(uintptr(windows.GetCurrentThreadId()), 0, 0, 0)
	getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
}

func namedEvent(name string, manual bool) windows.Handle {
	text, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0
	}
	reset := uint32(0)
	if manual {
		reset = 1
	}
	handle, _ := windows.CreateEvent(nil, reset, 0, text)
	return handle
}

func startGuard(tun *tunnel, stop <-chan struct{}) {
	alive, carrying := namedEvent(aliveEvent, false), namedEvent(carryEvent, true)
	if alive == 0 || carrying == 0 {
		return
	}
	windows.ResetEvent(carrying)
	exe, err := os.Executable()
	if err != nil {
		return
	}
	watcher := exec.Command(exe, append([]string{guardFlag, strconv.Itoa(os.Getpid())}, os.Args[1:]...)...)
	watcher.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	bequeath(watcher)
	if err := watcher.Start(); err != nil {
		fmt.Printf("guard    not started: %v\n", err)
		return
	}
	watcher.Process.Release()

	go func() {
		tick := time.NewTicker(beatEvery)
		defer tick.Stop()
		for {
			if tun.mu.TryLock() {
				up := tun.running
				tun.mu.Unlock()
				if up {
					windows.SetEvent(carrying)
				} else {
					windows.ResetEvent(carrying)
				}
				windows.SetEvent(alive)
			}
			select {
			case <-stop:
				return
			case <-tick.C:
			}
		}
	}()
}

func standGuard(args []string) {
	pid, err := strconv.Atoi(args[0])
	exe, err2 := os.Executable()
	if err != nil || err2 != nil {
		return
	}
	rest := args[1:]

	client, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(client)
	alive, carrying := namedEvent(aliveEvent, false), namedEvent(carryEvent, true)
	if alive == 0 || carrying == 0 {
		return
	}

	for {
		which, err := windows.WaitForMultipleObjects([]windows.Handle{client, alive}, false, guardWait)
		switch {
		case err != nil, which == windows.WAIT_OBJECT_0:
			return
		case which == windows.WAIT_OBJECT_0+1:
			continue
		}

		up, _ := windows.WaitForSingleObject(carrying, 0)
		windows.TerminateProcess(client, 1)
		windows.WaitForSingleObject(client, 5000)
		if up == windows.WAIT_OBJECT_0 && !slices.Contains(rest, "-connect") {
			rest = append(rest, "-connect")
		}
		start(exe, rest, 0)
		return
	}
}
