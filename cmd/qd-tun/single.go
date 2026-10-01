//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	instanceMutex = `Global\qdClient`
	instanceEvent = `Global\qdClientShow`
)

func claimInstance() (bool, func()) {
	name, err := windows.UTF16PtrFromString(instanceMutex)
	if err != nil {
		return true, func() {}
	}

	handle, err := windows.CreateMutex(nil, false, name)
	if handle == 0 {
		return true, func() {}
	}
	if err == windows.ERROR_ALREADY_EXISTS {
		windows.CloseHandle(handle)
		return false, func() {}
	}
	return true, func() { windows.CloseHandle(handle) }
}

func knock() bool {
	name, err := windows.UTF16PtrFromString(instanceEvent)
	if err != nil {
		return false
	}
	handle, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE|windows.SYNCHRONIZE, false, name)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	if windows.SetEvent(handle) != nil {
		return false
	}
	time.Sleep(knockWait)
	if state, _ := windows.WaitForSingleObject(handle, 0); state != uint32(windows.WAIT_OBJECT_0) {
		return true
	}
	fmt.Printf("single   the running client does not answer, ending it and taking its place\n")
	endOthers()
	return false
}

const knockWait = 1500 * time.Millisecond

func endOthers() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	mine := filepath.Base(exe)
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if entry.ProcessID == uint32(os.Getpid()) || !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), mine) {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, entry.ProcessID)
		if err != nil {
			continue
		}
		windows.TerminateProcess(h, 1)
		windows.WaitForSingleObject(h, 5000)
		windows.CloseHandle(h)
	}
}

func answerKnocks(show func(), stop <-chan struct{}) {
	name, err := windows.UTF16PtrFromString(instanceEvent)
	if err != nil {
		return
	}
	handle, _ := windows.CreateEvent(nil, 0, 0, name)
	if handle == 0 {
		return
	}
	defer windows.CloseHandle(handle)

	for {
		select {
		case <-stop:
			return
		default:
		}

		state, err := windows.WaitForSingleObject(handle, 500)
		if err != nil {
			return
		}
		if state == uint32(windows.WAIT_OBJECT_0) {
			show()
		}
	}
}
