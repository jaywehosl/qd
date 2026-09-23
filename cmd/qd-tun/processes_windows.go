//go:build windows

package main

import (
	"strings"

	"golang.org/x/sys/windows"
)

func snapshotProcesses() []processInfo {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return []processInfo{}
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafeSizeof(entry))

	out := []processInfo{}
	if err := windows.Process32First(snap, &entry); err != nil {
		return out
	}
	for {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if name != "" && !strings.EqualFold(name, "System") {
			path := lookupProcess(entry.ProcessID).path
			out = append(out, processInfo{
				Name: name, Path: path, Icon: processIcon(path), PID: int(entry.ProcessID),
			})
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	return out
}
