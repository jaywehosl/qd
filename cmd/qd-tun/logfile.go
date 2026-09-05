//go:build windows

package main

import (
	"log"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

const logSizeCap = 2 << 20

func redirectOutput(statePath string) {
	if hasConsole() {
		return
	}

	path := filepath.Join(filepath.Dir(statePath), "client.log")
	if info, err := os.Stat(path); err == nil && info.Size() > logSizeCap {
		os.Remove(path)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	os.Stdout = f
	os.Stderr = f
	// log держит writer, захваченный при инициализации пакета: подмены os.Stderr
	// ему мало, и всё, что движок пишет через log, улетало в никуда.
	log.SetOutput(f)
	// А runtime пишет панику прямо в дескриптор 2, мимо любых переменных: без
	// подмены самого дескриптора падение клиента не оставляло следов вообще.
	windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd()))
	windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(f.Fd()))
}

func hasConsole() bool {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow")
	hwnd, _, _ := proc.Call()
	return hwnd != 0
}
