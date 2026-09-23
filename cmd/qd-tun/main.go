//go:build windows || linux

package main

import "sync/atomic"

var routeByProcess atomic.Pointer[procRouter]
