package clientapi

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/quic-diver/internal/adblock"
	"github.com/jaywehosl/quic-diver/internal/clientstate"
)

type Visits struct {
	db   *clientstate.DB
	list *adblock.List

	on   atomic.Bool
	ch   chan string
	done chan struct{}
}

func NewVisits(db *clientstate.DB, list *adblock.List, adblockOn bool) *Visits {
	v := &Visits{
		db:   db,
		list: list,
		ch:   make(chan string, 512),
		done: make(chan struct{}),
	}
	v.on.Store(adblockOn)

	go v.tally()
	return v
}

func (v *Visits) SetAdblock(on bool) { v.on.Store(on) }

func (v *Visits) Query(name string) bool {
	if v.on.Load() && v.list.Blocked(name) {
		return true
	}

	if !reverseLookup(name) {
		select {
		case v.ch <- name:
		default:
		}
	}
	return false
}

func reverseLookup(name string) bool {
	return strings.HasSuffix(name, ".in-addr.arpa") || strings.HasSuffix(name, ".ip6.arpa")
}

func (v *Visits) Close() {
	close(v.ch)
	<-v.done
}

const siteFlush = 30 * time.Second

func (v *Visits) tally() {
	defer close(v.done)
	pending := map[string]int{}
	flush := func() {
		if len(pending) == 0 {
			return
		}
		v.db.NoteSites(pending)
		pending = map[string]int{}
	}
	tick := time.NewTicker(siteFlush)
	defer tick.Stop()
	for {
		select {
		case name, ok := <-v.ch:
			if !ok {
				flush()
				return
			}
			pending[name]++
		case <-tick.C:
			flush()
		}
	}
}
