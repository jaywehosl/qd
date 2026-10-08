package qdmobile

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/qd/internal/pace"
	"github.com/jaywehosl/qd/internal/roads"
)

const (
	settle     = 250 * time.Millisecond
	lingerFor  = 30 * time.Second
	lingerStep = 5 * time.Second
	glanceWait = pace.GlanceWait
)

var (
	relinking atomic.Bool
	again     atomic.Bool
	orphaned  atomic.Bool
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
	under := c.pathTag
	c.mu.Unlock()

	say("net: tag=%s same=%v first=%v running=%v", tag, same, first, running)

	if same {
		return
	}
	roads.Forget()
	if !running && c.wanted.Load() {
		select {
		case nudge <- struct{}{}:
		default:
		}
		return
	}
	if first || !running {
		return
	}
	if tag == under {
		say("net: the system is back on the network the tunnel never left")
		return
	}

	go c.follow(tag)
}

func (c *Client) follow(tag string) {
	c.mu.Lock()
	live := c.live
	c.mu.Unlock()
	stays := live != nil && live.CanMigrate() && live.Reaches(context.Background())
	if !orphaned.Swap(false) && stays && c.pathAnswersIn(context.Background(), glanceWait) {
		say("net: the path in use still answers, staying on it")
		for waited := time.Duration(0); waited < lingerFor; waited += lingerStep {
			time.Sleep(lingerStep)

			c.mu.Lock()
			wanted := c.running && c.netTag == tag && c.pathTag != tag
			c.mu.Unlock()
			if !wanted {
				return
			}
			if !c.pathAnswersIn(context.Background(), glanceWait) {
				break
			}
		}
		say("net: moving the tunnel over to %s", tag)
	}
	c.migrate(context.Background())
}

func (c *Client) NetworkLost(id string) {
	c.mu.Lock()
	under, best, running := c.pathTag, c.netTag, c.running
	c.mu.Unlock()

	if !running || !strings.HasSuffix(under, ":"+id) {
		return
	}
	say("net: the network under the tunnel is gone")
	if best == under {
		orphaned.Store(true)
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
			go c.comeBack()
		}

		if !again.Load() {
			return
		}
	}
}
