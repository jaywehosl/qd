package quiet

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const (
	window   = 10 * time.Second
	keepMost = 1024
)

type shape struct {
	first  time.Time
	extra  int
	sample string
}

type writer struct {
	to   io.Writer
	mu   sync.Mutex
	seen map[string]*shape
}

func Writer(to io.Writer) io.Writer {
	w := &writer{to: to, seen: map[string]*shape{}}
	go w.drain()
	return w
}

func (w *writer) Write(p []byte) (int, error) {
	line := strings.TrimRight(string(p), "\n")
	key := shapeOf(line)
	now := time.Now()

	w.mu.Lock()
	held := w.seen[key]
	if held != nil && now.Sub(held.first) < window {
		held.extra++
		w.mu.Unlock()
		return len(p), nil
	}
	said := ""
	if held != nil && held.extra > 0 {
		said = repeated(held)
	}
	if len(w.seen) >= keepMost {
		said += w.flushLocked(now, true)
	}
	w.seen[key] = &shape{first: now, sample: line}
	w.mu.Unlock()

	if said != "" {
		io.WriteString(w.to, said)
	}
	if _, err := w.to.Write(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *writer) drain() {
	tick := time.NewTicker(window)
	defer tick.Stop()
	for now := range tick.C {
		w.mu.Lock()
		said := w.flushLocked(now, false)
		w.mu.Unlock()
		if said != "" {
			io.WriteString(w.to, said)
		}
	}
}

func (w *writer) flushLocked(now time.Time, all bool) string {
	var out strings.Builder
	for key, held := range w.seen {
		if !all && now.Sub(held.first) < window {
			continue
		}
		if held.extra > 0 {
			out.WriteString(repeated(held))
		}
		delete(w.seen, key)
	}
	return out.String()
}

func repeated(held *shape) string {
	return fmt.Sprintf("%s (and %d more like it in %s)\n", held.sample, held.extra, window)
}

func shapeOf(line string) string {
	var out strings.Builder
	digits := false
	for _, r := range line {
		if r >= '0' && r <= '9' {
			if !digits {
				out.WriteByte('#')
			}
			digits = true
			continue
		}
		digits = false
		out.WriteRune(r)
	}
	return out.String()
}
