//go:build windows

package main

import "syscall"

var iphlpapi = syscall.NewLazyDLL("iphlpapi.dll")

const (
	afInet  = 2
	afInet6 = 23
)
