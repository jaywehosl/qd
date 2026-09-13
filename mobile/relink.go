package qdmobile

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/quic-diver/internal/roads"
)

const settle = 250 * time.Millisecond

var (
	relinking atomic.Bool
	again     atomic.Bool
)

func (c *Client) NetworkChanged(tag string) {
	if tag == "" {
		return
	}

	c.mu.Lock()
	first := c.netTag == ""
	same := c.netTag == tag
	c.netTag = tag
	running := c.running
	c.mu.Unlock()

	say("net: tag=%s same=%v first=%v running=%v", tag, same, first, running)

	if same {
		return
	}
	roads.Forget()
	if first || !running {
		return
	}

	go c.migrate(context.Background())
}

func (c *Client) relink() {
	if !relinking.CompareAndSwap(false, true) {
		again.Store(true)
		return
	}
	defer relinking.Store(false)

	for {
		again.Store(false)

		if !c.Running() {
			return
		}

		c.stopCarry()
		if err := c.api.Connect(); err != nil {
			say("relink: could not come back: %v", err)
		}

		if !again.Load() {
			return
		}
	}
}
