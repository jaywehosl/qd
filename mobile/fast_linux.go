//go:build linux

package qdmobile

import (
	"golang.org/x/sys/unix"
)

func runFast() {
	tid := unix.Gettid()
	for _, nice := range []int{-10, -5, -2} {
		if err := unix.Setpriority(unix.PRIO_PROCESS, tid, nice); err == nil {
			return
		}
	}
}
