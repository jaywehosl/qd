package clientapi

import "sync/atomic"

type Adblock struct{ on atomic.Bool }

func NewAdblock(on bool) *Adblock {
	a := &Adblock{}
	a.on.Store(on)
	return a
}

func (a *Adblock) Set(on bool) { a.on.Store(on) }

func (a *Adblock) On() bool { return a.on.Load() }
