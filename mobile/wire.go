package qdmobile

import (
	"encoding/hex"

	"github.com/jaywehosl/quic-diver/internal/clientapi"
	"github.com/jaywehosl/quic-diver/internal/qwire"
)

func (c *Client) keeper() func(fd uintptr) {
	return func(fd uintptr) {
		c.mu.Lock()
		protector := c.protector
		c.mu.Unlock()

		if protector == nil {
			return
		}
		if !protector.Protect(int(fd)) && c.Running() {
			say("socket: the system refused to keep fd %d out of the tunnel", fd)
		}
	}
}

func (c *Client) wire() *qwire.Dialer {
	c.mu.Lock()
	held := c.talk
	c.mu.Unlock()
	if held != nil {
		return held
	}

	fresh := qwire.NewKept(c.keeper())
	fresh.SetToken(c.netKeyHex())

	c.mu.Lock()
	if c.talk == nil {
		c.talk = fresh
	}
	held = c.talk
	c.mu.Unlock()

	return held
}

func (p platform) Wire() clientapi.Asker { return p.c.wire() }

func (c *Client) netKeyHex() string {
	c.mu.Lock()
	key := c.key
	c.mu.Unlock()

	if key == nil {
		return ""
	}
	return hex.EncodeToString(key[:])
}

func (c *Client) tellWire() {
	c.mu.Lock()
	held := c.talk
	c.mu.Unlock()

	if held != nil {
		held.SetToken(c.netKeyHex())
	}
}
