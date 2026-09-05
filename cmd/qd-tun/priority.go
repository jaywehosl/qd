//go:build windows

package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	aboveNormalPriority = 0x00008000

	processPowerThrottling = 4

	powerThrottlingVersion    = 1
	throttleExecutionSpeed    = 0x1
	throttleIgnoreTimerRes    = 0x4
	powerThrottlingStateBytes = 12
)

type powerThrottlingState struct {
	Version     uint32
	ControlMask uint32
	StateMask   uint32
}

func standFull() {
	me := windows.CurrentProcess()

	if err := windows.SetPriorityClass(me, aboveNormalPriority); err != nil {
		fmt.Printf("cpu      could not raise the priority: %v\n", err)
	}

	state := powerThrottlingState{
		Version:     powerThrottlingVersion,
		ControlMask: throttleExecutionSpeed | throttleIgnoreTimerRes,
		StateMask:   0,
	}
	if err := setProcessInformation(me, processPowerThrottling,
		unsafe.Pointer(&state), powerThrottlingStateBytes); err != nil {
		fmt.Printf("cpu      could not turn off power throttling: %v\n", err)
		return
	}
	fmt.Printf("cpu      full speed asked for, background throttling off\n")
}

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procSetProcessInformation = kernel32.NewProc("SetProcessInformation")
	procSetThreadPriority     = kernel32.NewProc("SetThreadPriority")
	winmm                     = windows.NewLazySystemDLL("winmm.dll")
	procTimeBeginPeriod       = winmm.NewProc("timeBeginPeriod")
	procTimeEndPeriod         = winmm.NewProc("timeEndPeriod")
)

func setProcessInformation(h windows.Handle, class uint32, info unsafe.Pointer, size uint32) error {
	r, _, err := procSetProcessInformation.Call(
		uintptr(h), uintptr(class), uintptr(info), uintptr(size))
	if r == 0 {
		return err
	}
	return nil
}

func sharpTimers() func() {
	if r, _, err := procTimeBeginPeriod.Call(1); r != 0 {
		fmt.Printf("cpu      timers stay coarse: %v\n", err)
		return func() {}
	}
	return func() { procTimeEndPeriod.Call(1) }
}

func runFast() {
	procSetThreadPriority.Call(uintptr(windows.CurrentThread()), aboveNormalThread)
}

const aboveNormalThread = 1
