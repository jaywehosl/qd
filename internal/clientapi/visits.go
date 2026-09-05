package clientapi

import (
	"strings"
	"sync/atomic"

	"github.com/jaywehosl/quic-diver/internal/adblock"
	"github.com/jaywehosl/quic-diver/internal/clientstate"
)

type Visits struct {
	db   *clientstate.DB
	list *adblock.List

	on      atomic.Bool
	blocked atomic.Uint64
	ch      chan string
	done    chan struct{}
}

func NewVisits(db *clientstate.DB, list *adblock.List, adblockOn bool) *Visits {
	v := &Visits{
		db:   db,
		list: list,
		ch:   make(chan string, 512),
		done: make(chan struct{}),
	}
	v.on.Store(adblockOn)

	go func() {
		defer close(v.done)
		for name := range v.ch {
			v.db.NoteSite(name)
		}
	}()
	return v
}

func (v *Visits) SetAdblock(on bool) { v.on.Store(on) }

func (v *Visits) Blocked() uint64 { return v.blocked.Load() }

func (v *Visits) Query(name string) bool {
	if v.on.Load() && v.list.Blocked(name) {
		v.blocked.Add(1)
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
