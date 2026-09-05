//go:build windows

package main

import (
	"golang.org/x/sys/windows"
)

const (
	instanceMutex = `Global\QuicDiverClient`
	instanceEvent = `Global\QuicDiverClientShow`
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
	handle, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	return windows.SetEvent(handle) == nil
}

func answerKnocks(show func(), stop <-chan struct{}) {
	name, err := windows.UTF16PtrFromString(instanceEvent)
	if err != nil {
		return
	}
	handle, err := windows.CreateEvent(nil, 0, 0, name)
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
