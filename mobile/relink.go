package qdmobile

import (
	"context"
	"sync/atomic"
	"time"
)

const settle = 250 * time.Millisecond

var (
	relinking atomic.Bool
	again     atomic.Bool
	relinks   atomic.Uint64
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
	if first || !running {
		return
	}

	// Сеть сменилась — переезжаем, а не пересобираем: сессия на узле остаётся
	// той же, живые соединения в приложениях не рвутся.
	go c.migrate(context.Background())
}

// relink пересобирает туннель целиком. Нужен там, где переезд не поможет: набор
// приложений в VpnService задаётся при создании, поэтому смена правил требует
// нового устройства, а не нового пути.
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
		relinks.Add(1)

		c.stopCarry()
		if err := c.api.Connect(); err != nil {
			say("relink: could not come back: %v", err)
		}

		if !again.Load() {
			return
		}
	}
}

func (c *Client) Relinks() int {
	return int(relinks.Load())
}
