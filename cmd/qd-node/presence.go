//go:build linux

package main

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type presence struct {
	sample func() (map[uint32]seen, error)

	mu    sync.RWMutex
	stint map[uint32]*stint
}

type seen struct {
	LastSeen int64
	LastPing int64
	Transit  bool
	Client   string
	Up       uint64
	Down     uint64
	PktUp    uint64
	PktDown  uint64
}

type stint struct {
	Joined    bool
	Since     int64
	LastHeard int64
	Checked   int64
	Client    string
	Device    string
	Seen      []address
}

type address struct {
	IP        string `json:"ip"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastOnline"`
}

const gone = 90 * time.Second

func watchPresence(sample func() (map[uint32]seen, error)) *presence {
	p := &presence{sample: sample, stint: map[uint32]*stint{}}
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for range t.C {
			p.tick()
		}
	}()
	return p
}

func (p *presence) Joining(id uint32, fingerprint string) {
	now := time.Now().UnixMilli()

	p.mu.Lock()
	defer p.mu.Unlock()

	held := p.stint[id]
	if held == nil {
		held = &stint{Seen: []address{}}
		p.stint[id] = held
	}
	if !held.Joined {
		held.Since = now
		held.Seen = []address{}
	}
	held.Joined = true
	held.LastHeard = now
	held.Checked = now
	if fingerprint != "" {
		held.Device = fingerprint
	}
}

func (p *presence) Leaving(id uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if held := p.stint[id]; held != nil {
		held.Joined = false
		held.Since = 0
		held.LastHeard = time.Now().UnixMilli()
	}
}

func (p *presence) Checked(id uint32, fingerprint string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	held := p.stint[id]
	if held == nil {
		held = &stint{Seen: []address{}}
		p.stint[id] = held
	}
	held.Checked = time.Now().UnixMilli()
	if fingerprint != "" {
		held.Device = fingerprint
	}
}

func (p *presence) tick() {
	live, err := p.sample()
	if err != nil {
		return
	}

	now := time.Now().UnixMilli()
	cutoff := now - gone.Milliseconds()

	p.mu.Lock()
	defer p.mu.Unlock()

	for id, s := range live {
		held := p.stint[id]
		if held == nil {
			held = &stint{Seen: []address{}, Joined: true, Since: now, LastHeard: now}
			p.stint[id] = held
		}
		if !held.Joined {
			continue
		}

		held.LastHeard = now

		if held.Client != "" && s.Client != "" && held.Client != s.Client {
			fmt.Printf("roam       session %d moved from %s to %s\n", id, held.Client, s.Client)
		}
		held.Client = s.Client
		if s.Client == "" {
			continue
		}

		host := s.Client
		if colon := lastColon(host); colon > 0 {
			host = host[:colon]
		}

		found := false
		for i := range held.Seen {
			if held.Seen[i].IP == host {
				held.Seen[i].LastSeen = now
				found = true
				break
			}
		}
		if !found {
			held.Seen = append(held.Seen, address{IP: host, FirstSeen: now, LastSeen: now})
		}
	}

	for id, held := range p.stint {
		if _, carried := live[id]; carried {
			continue
		}
		if held.Checked > cutoff {
			continue
		}
		delete(p.stint, id)
	}
}

func (p *presence) of(id uint32) (since, lastHeard, checked int64, device string, seen []address) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	held := p.stint[id]
	if held == nil {
		return 0, 0, 0, "", []address{}
	}

	seen = make([]address, len(held.Seen))
	copy(seen, held.Seen)
	sort.Slice(seen, func(i, j int) bool { return seen[i].LastSeen > seen[j].LastSeen })

	if !held.Joined {
		return 0, held.LastHeard, held.Checked, held.Device, seen
	}
	return held.Since, held.LastHeard, held.Checked, held.Device, seen
}

func (p *presence) Live() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	live := 0
	for _, held := range p.stint {
		if held.Joined {
			live++
		}
	}
	return live
}

func lastColon(text string) int {
	for i := len(text) - 1; i >= 0; i-- {
		if text[i] == ':' {
			return i
		}
	}
	return -1
}
