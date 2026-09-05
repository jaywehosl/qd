//go:build windows

package wintun

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed wintun.dll
var payload []byte

func unpack() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "wintun.dll"
	}
	dir = filepath.Join(dir, "QuicDiver")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "wintun.dll"
	}

	path := filepath.Join(dir, "wintun.dll")
	if same(path) {
		return path
	}

	if err := os.WriteFile(path, payload, 0o600); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return path
		}
		return "wintun.dll"
	}
	return path
}

func same(path string) bool {
	held, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	mine := sha256.Sum256(payload)
	theirs := sha256.Sum256(held)
	return bytes.Equal(mine[:], theirs[:])
}
