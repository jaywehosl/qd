//go:build windows || linux

package main

import (
	"sort"
	"strings"
	"sync"
	"time"
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
	sort.Slice(items, func(i, j int) bool {
		if items[i].Connections != items[j].Connections {
			return items[i].Connections > items[j].Connections
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	procCache.at = time.Now()
	procCache.items = items
	return items
}
