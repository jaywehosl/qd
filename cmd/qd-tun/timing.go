//go:build windows

package main

import (
	"fmt"
	"strings"
	"time"
)

type steps struct {
	t0    time.Time
	last  time.Time
	marks []mark
}

type mark struct {
	name string
	took time.Duration
}

func newSteps() *steps {
	now := time.Now()
	return &steps{t0: now, last: now}
}

func (s *steps) mark(name string) {
	if s == nil {
		return
	}
	now := time.Now()
	s.marks = append(s.marks, mark{name: name, took: now.Sub(s.last)})
	s.last = now
}

func (s *steps) report(total time.Duration) {
	if s == nil || len(s.marks) == 0 {
		return
	}
	var b strings.Builder
	for _, m := range s.marks {
		ms := float64(m.took.Microseconds()) / 1000
		if ms < 1 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("  ")
		}
		fmt.Fprintf(&b, "%s %.0f", m.name, ms)
	}
	if b.Len() == 0 {
		return
	}
	fmt.Printf("timing   %s  (ms of %d)\n", b.String(), total.Milliseconds())
}
