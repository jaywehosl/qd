//go:build linux

package qdmobile

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func holdStderr(dir string) {
	file, err := os.OpenFile(filepath.Join(dir, "qd.crash"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	_ = unix.Dup2(int(file.Fd()), 2)
	os.Stderr = file
}
