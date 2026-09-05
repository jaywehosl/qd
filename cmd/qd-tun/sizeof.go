//go:build windows

package main

import "unsafe"

func unsafeSizeof[T any](v T) uintptr {
	return unsafe.Sizeof(v)
}
