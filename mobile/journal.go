package qdmobile

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const journalCap = 512 << 10

type journal struct {
	mu   sync.Mutex
	file *os.File
	path string
	size int64
}

var (
	book  atomic.Pointer[journal]
	where atomic.Pointer[string]
	loud  atomic.Bool
)

func markJournal(dir string) {
	path := filepath.Join(dir, "qd.log")
	where.Store(&path)
	catchStdout()
	holdStderr(dir)
}

func (c *Client) Verbose(on bool) {
	loud.Store(on)
	spot := where.Load()
	if spot == nil {
		return
	}

	if !on {
		if kept := book.Swap(nil); kept != nil {
			kept.mu.Lock()
			if kept.file != nil {
				kept.file.Close()
			}
			kept.mu.Unlock()
		}
		os.Remove(*spot)
		return
	}
	if book.Load() != nil {
		return
	}

	file, err := os.OpenFile(*spot, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}

	kept := &journal{file: file, path: *spot}
	if info, err := file.Stat(); err == nil {
		kept.size = info.Size()
	}
	book.Store(kept)
}

func whoCalled() string {
	var out []string
	for depth := 2; depth < 8; depth++ {
		pc, _, line, ok := runtime.Caller(depth)
		if !ok {
			break
		}
		name := runtime.FuncForPC(pc).Name()
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		out = append(out, fmt.Sprintf("%s:%d", name, line))
	}
	return strings.Join(out, " < ")
}

func say(format string, args ...any) {
	kept := book.Load()
	if kept == nil {
		return
	}

	kept.mu.Lock()
	defer kept.mu.Unlock()
	if kept.file == nil {
		return
	}

	if kept.size > journalCap {
		kept.file.Truncate(0)
		kept.file.Seek(0, 0)
		kept.size = 0
	}

	written, _ := fmt.Fprintf(kept.file, "%s %s\n",
		time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	kept.size += int64(written)
}

func (c *Client) Say(text string) {
	say("%s", text)
}

func (c *Client) LogPath() string {
	kept := book.Load()
	if kept == nil {
		return ""
	}
	return kept.path
}

var caught sync.Once

func catchStdout() {
	caught.Do(func() {
		r, w, err := os.Pipe()
		if err != nil {
			return
		}
		os.Stdout = w
		go func() {
			buf := make([]byte, 4<<10)
			rest := ""
			for {
				n, err := r.Read(buf)
				if n > 0 {
					rest += string(buf[:n])
					for {
						cut := strings.IndexByte(rest, '\n')
						if cut < 0 {
							break
						}
						if line := strings.TrimRight(rest[:cut], "\r"); line != "" {
							say("%s", line)
						}
						rest = rest[cut+1:]
					}
				}
				if err != nil {
					return
				}
			}
		}()
	})
}
