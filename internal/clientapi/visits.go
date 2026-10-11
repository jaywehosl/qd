package clientapi

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/qd/internal/clientstate"
)

type Visits struct {
	db *clientstate.DB

	on   atomic.Bool
	ch   chan string
	done chan struct{}
}

func NewVisits(db *clientstate.DB, adblockOn bool) *Visits {
	v := &Visits{
		db:   db,
		ch:   make(chan string, 512),
		done: make(chan struct{}),
	}
	v.on.Store(adblockOn)

	go v.tally()
	return v
}

func (v *Visits) SetAdblock(on bool) { v.on.Store(on) }

func (v *Visits) Adblock() bool { return v.on.Load() }

func (v *Visits) Note(name string) {
	if reverseLookup(name) {
		return
	}
	select {
	case v.ch <- name:
	default:
	}
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
