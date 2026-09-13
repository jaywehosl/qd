//go:build windows

package main

import "syscall"

var iphlpapi = syscall.NewLazyDLL("iphlpapi.dll")

const (
	afUnspec = 0
	afInet   = 2
	afInet6  = 23
)
