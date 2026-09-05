//go:build !windows

package carry

import "syscall"

const noBuffers = syscall.ENOBUFS
