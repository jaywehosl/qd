package qdmobile

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/qd/internal/qcli"
)

var moving atomic.Bool

var lastSwitch atomic.Pointer[time.Time]

const switchRest = 90 * time.Second

var switches atomic.Int32

func switchPause() time.Duration { return switchRest << min(switches.Load(), 5) }

func (c *Client) comeOver() {
	now := time.Now()
	lastSwitch.Store(&now)
	switches.Add(1)
	say("roam: quic answers on this path after all, coming back over it")
	go c.lost()
}

func (c *Client) watch(ctx context.Context, stop <-chan struct{}) {
	tick := time.NewTicker(deafStep)
	defer tick.Stop()

	c.mu.Lock()
	live := c.live
	c.mu.Unlock()
	if live == nil {
		return
	}

	was := live.Stats()
	deaf := time.Time{}
	heardAt := time.Now()
	last := time.Now()
	better := live.Better()
	var later <-chan time.Time
	if !live.OverTCP() {
		switches.Store(0)
	}

	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-better:
			better = nil
			if held := lastSwitch.Load(); held != nil && time.Since(*held) < switchPause() {
				later = time.After(switchPause() - time.Since(*held))
				continue
			}
			c.comeOver()
			return
		case <-later:
			c.comeOver()
			return
		case <-tick.C:
		}

		stood := time.Since(last)
		last = time.Now()

		if !live.Alive() {
			say("roam: the session is gone, coming back")
			go c.lost()
			return
		}

		if stood > goneFor {
			say("roam: the process stood still for %s, the node has dropped the session by now", stood.Round(time.Second))
			go c.lost()
			return
		}

		if stood > frozenFor {
			say("roam: the process stood still for %s, asking the path", stood.Round(time.Second))
			if !c.pathAnswers(ctx) {
				go c.lost()
				return
			}
			was, deaf, heardAt = live.Stats(), time.Time{}, time.Now()
			continue
		}

		now := live.Stats()
		heard := now.Heard != was.Heard
		spoke := now.Out != was.Out
		if now.Back != was.Back {
			heardAt = time.Now()
		}
		was = now

		if heard {
			if !deaf.IsZero() {
				say("roam: the path answers again")
			}
			deaf = time.Time{}
			continue
		}

		if time.Since(heardAt) > silenceFor {
			say("roam: nothing has come back for %s, asking the path", silenceFor)
			if !c.pathAnswers(ctx) {
				go c.lost()
				return
			}
			was, deaf, heardAt = live.Stats(), time.Time{}, time.Now()
			continue
		}

		if !spoke {
			deaf = time.Time{}
			continue
		}

		if deaf.IsZero() {
			deaf = time.Now()
			continue
		}
		if time.Since(deaf) < deafFor {
			continue
		}
		if time.Since(deaf) < deafFor+patience {
			say("roam: nothing comes back for %s, trying to migrate in place", deafFor)
			c.migrate(ctx)
			deaf = time.Now().Add(-deafFor)
			continue
		}

		say("roam: the node stopped answering for %s, giving up on this path", patience)
		go c.lost()
		return
	}
}

func (c *Client) pathAnswers(ctx context.Context) bool { return c.pathAnswersIn(ctx, askWait) }

func (c *Client) pathAnswersIn(ctx context.Context, wait time.Duration) bool {
	c.mu.Lock()
	live := c.live
	c.mu.Unlock()
	if live == nil {
		return false
	}

	round, done := context.WithTimeout(ctx, wait)
	err := live.Ask(round)
	done()
	if err == nil {
		return true
	}
	say("roam: the path does not answer: %v", err)
	return false
}

func (c *Client) migrate(ctx context.Context) {
	if !moving.CompareAndSwap(false, true) {
		return
	}
	defer moving.Store(false)

	c.mu.Lock()
	live := c.live
	c.mu.Unlock()
	if live == nil {
		return
	}

	if !live.CanMigrate() {
		say("roam: this path does not migrate, bringing the tunnel up again")
		go c.lost()
		return
	}

	for try := 1; try <= tries; try++ {
		round, done := context.WithTimeout(ctx, moveWait)
		err := live.Rebind(round)
		done()

		if err == nil {
			say("roam: the path moved, the tunnel migrated in place")
			c.mu.Lock()
			c.pathTag = c.netTag
			c.mu.Unlock()
			c.wire().Reset()
			go c.prove(live)
			return
		}
		if ctx.Err() != nil {
			return
		}
		say("roam: migration attempt %d of %d failed: %v", try, tries, err)

		select {
		case <-ctx.Done():
			return
		case <-time.After(pause):
		}
	}

	say("roam: the path did not move, coming back through a fresh dial")
	go c.lost()
}

func (c *Client) prove(moved *qcli.Tunnel) {
	for _, wait := range []time.Duration{time.Second, 3 * time.Second, 4 * time.Second} {
		time.Sleep(wait)
		c.mu.Lock()
		held := c.running && c.live == moved
		c.mu.Unlock()
		if !held {
			return
		}
		if !c.pathAnswers(context.Background()) {
			say("roam: the moved path went quiet, coming back through a fresh dial")
			go c.lost()
			return
		}
	}
}

const (
	deafStep   = 3 * time.Second
	deafFor    = 20 * time.Second
	patience   = 45 * time.Second
	frozenFor  = 30 * time.Second
	goneFor    = 75 * time.Second
	silenceFor = 60 * time.Second
	askWait    = 3 * time.Second
	tries      = 1
	moveWait   = 1500 * time.Millisecond
	glanceWait = 1500 * time.Millisecond
	pause      = 1 * time.Second
)
