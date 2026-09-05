//go:build linux

package main

import (
	"bufio"
	"os"
	"strings"
	"sync"
	"time"
)

type logRing struct {
	mu    sync.RWMutex
	lines []string
	max   int
}

var realStderr = os.Stderr

func captureOutput(max int) *logRing {
	ring := &logRing{max: max}

	read, write, err := os.Pipe()
	if err != nil {
		return ring
	}
	real := os.Stdout
	realStderr = real
	os.Stdout = write
	os.Stderr = write

	go func() {
		scanner := bufio.NewScanner(read)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			real.WriteString(line + "\n")
			ring.add(time.Now().UTC().Format("2006/01/02 15:04:05") + " " + level(line) + " - " + line)
		}
	}()
	return ring
}

func level(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "fatal"):
		return "ERROR"
	case strings.Contains(lower, "not listening") || strings.Contains(lower, "disabled") || strings.Contains(lower, "dropped"):
		return "WARNING"
	}
	return "INFO"
}

func (r *logRing) add(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.lines) == r.max {
		copy(r.lines, r.lines[1:])
		r.lines = r.lines[:r.max-1]
	}
	r.lines = append(r.lines, line)
}

func (r *logRing) Tail(n int, level string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	floor := rank(level)
	kept := make([]string, 0, len(r.lines))
	for _, line := range r.lines {
		if rank(levelOf(line)) >= floor {
			kept = append(kept, line)
		}
	}

	if n <= 0 || n > len(kept) {
		n = len(kept)
	}
	return kept[len(kept)-n:]
}

func (r *logRing) Forget(level string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	floor := rank(level)
	kept := make([]string, 0, len(r.lines))
	for _, line := range r.lines {
		if rank(levelOf(line)) < floor {
			kept = append(kept, line)
		}
	}

	dropped := len(r.lines) - len(kept)
	r.lines = kept
	return dropped
}

func rank(level string) int {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return 0
	case "NOTICE":
		return 2
	case "WARNING", "WARN":
		return 3
	case "ERROR", "ERR":
		return 4
	}
	return 1
}

func levelOf(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return "INFO"
	}
	return fields[2]
}
