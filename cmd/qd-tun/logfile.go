//go:build windows

package main

import (
	"bufio"
	"io"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const logSizeCap = 2 << 20

var logDrained = make(chan struct{})

func redirectOutput(statePath string) {
	if hasConsole() {
		close(logDrained)
		return
	}
	path := filepath.Join(filepath.Dir(statePath), "client.log")
	os.MkdirAll(filepath.Dir(path), 0o755)
	if info, err := os.Stat(path); err == nil && info.Size() > logSizeCap {
		os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		close(logDrained)
		return
	}
	os.Stderr = f
	windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd()))

	r, w, err := os.Pipe()
	if err != nil {
		os.Stdout = f
		log.SetOutput(f)
		windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(f.Fd()))
		close(logDrained)
		return
	}
	os.Stdout = w
	log.SetFlags(0)
	log.SetOutput(w)
	windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(w.Fd()))
	go stampLines(r, f)
}

func stampLines(from io.Reader, into *os.File) {
	defer close(logDrained)
	lines := bufio.NewReaderSize(from, 64<<10)
	for {
		line, err := lines.ReadString('\n')
		if line != "" {
			if line[len(line)-1] != '\n' {
				line += "\n"
			}
			into.WriteString(time.Now().Format("02.01 15:04:05.000 ") + line)
		}
		if err != nil {
			return
		}
	}
}

func flushLog() {
	if os.Stdout != nil {
		os.Stdout.Close()
	}
	select {
	case <-logDrained:
	case <-time.After(time.Second):
	}
}

func hasConsole() bool {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow")
	hwnd, _, _ := proc.Call()
	return hwnd != 0
}
