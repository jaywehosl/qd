//go:build windows

package main

import (
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

type processInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path,omitempty"`
	Icon        string `json:"icon,omitempty"`
	PID         int    `json:"pid"`
	Connections int    `json:"connections"`
}

var procCache struct {
	mu    sync.Mutex
	at    time.Time
	items []processInfo
}

func runningProcesses() []processInfo {
	procCache.mu.Lock()
	defer procCache.mu.Unlock()

	if time.Since(procCache.at) < 5*time.Second && procCache.items != nil {
		return procCache.items
	}

	items := snapshotProcesses()
	procCache.at = time.Now()
	procCache.items = items
	return items
}

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
			// The path is what a rule keys on and what the icon is read from,
			// so it is worth the handle. System processes refuse it, and those
			// simply carry no path and no icon.
			path := lookupProcess(entry.ProcessID).path
			out = append(out, processInfo{
				Name: name, Path: path, Icon: processIcon(path), PID: int(entry.ProcessID),
			})
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Connections != out[j].Connections {
			return out[i].Connections > out[j].Connections
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}
