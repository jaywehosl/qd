package qdmobile

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/qd/internal/clientrun"
	"github.com/jaywehosl/qd/internal/pace"
)

var moving atomic.Bool

func (c *Client) watch(ctx context.Context, stop <-chan struct{}) {
	c.mu.Lock()
	live := c.live
	c.mu.Unlock()
	if live == nil {
		return
	}

	clientrun.Watch{
		Live:    live,
		Migrate: c.move,
		Lost: func(why error) {
			say("roam: %v, coming back", why)
			go c.lost()
		},
		Say: say,
	}.Run(ctx, stop)
}

func (c *Client) pathAnswers(ctx context.Context) bool { return c.pathAnswersIn(ctx, pace.AskWait) }

func (c *Client) pathAnswersIn(ctx context.Context, wait time.Duration) bool {
	c.mu.Lock()
	live := c.live
	c.mu.Unlock()
	if live == nil {
		return false
	}
	return clientrun.PathAnswers(ctx, live, wait, say)
}

func (c *Client) migrate(ctx context.Context) {
	if !c.move(ctx) && ctx.Err() == nil {
		go c.lost()
	}
}

func (c *Client) move(ctx context.Context) bool {
	if !moving.CompareAndSwap(false, true) {
		return true
	}
	defer moving.Store(false)

	c.mu.Lock()
	live := c.live
	c.mu.Unlock()
	if live == nil {
		return true
	}
	if !clientrun.Move(ctx, live, say) {
		return false
	}

	c.mu.Lock()
	c.pathTag = c.netTag
	c.mu.Unlock()
	c.wire().Reset()
	go func() {
		held := func() bool {
			c.mu.Lock()
			defer c.mu.Unlock()
			return c.running && c.live == live
		}
		if !clientrun.Prove(live, held, say) && held() {
			go c.lost()
		}
	}()
	return true
}
