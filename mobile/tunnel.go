package qdmobile

import (
	"sync/atomic"

	"github.com/jaywehosl/quic-diver/internal/clientapi"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
	"github.com/jaywehosl/quic-diver/internal/qsrv"
)

var exit atomic.Uint32

func (c *Client) announce(op string) {
	sub, err := c.db.Subscription()
	if err != nil || !sub.Imported {
		return
	}
	nodes, err := c.db.Nodes()
	if err != nil {
		return
	}
	heard := clientapi.Announce(op, nodes, c.api.Key(), sub.Key, c.device, c.wire())
	say("fleet: %s heard by %d of %d nodes", op, heard, len(nodes))
}

func (c *Client) applyExit(on bool) {
	tag := ""
	if on {
		exit.Store(uint32(qdcrypt.ExitEgress))
		tag = qsrv.AnyExit
	} else {
		exit.Store(uint32(qdcrypt.ExitLocal))
	}

	if c.marks != nil {
		c.marks.forget()
	}

	c.mu.Lock()
	live := c.live
	c.mu.Unlock()
	if live == nil {
		return
	}
	live.SetRoute(tag)
	live.Reroute()
}

func (c *Client) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func (c *Client) reroute() {
	c.mu.Lock()
	live := c.live
	c.mu.Unlock()
	if live != nil {
		live.Reroute()
	}
}
