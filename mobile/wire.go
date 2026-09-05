package qdmobile

import (
	"encoding/hex"

	"github.com/jaywehosl/quic-diver/internal/clientapi"
	"github.com/jaywehosl/quic-diver/internal/qwire"
)

// keeper помечает сокет как исключённый из туннеля. Android заворачивает в VPN
// весь трафик, включая наш собственный: без этой отметки разговор с узлом уходит
// в туннель, который сам же и поднимает.
func (c *Client) keeper() func(fd uintptr) {
	return func(fd uintptr) {
		c.mu.Lock()
		protector := c.protector
		c.mu.Unlock()

		if protector == nil {
			return
		}
		// Пока туннеля нет, отметка и не нужна: заворачивать трафик некуда.
		// Жаловаться на это значит шуметь на каждом обращении к узлу.
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

// netKeyHex — ключ сети, которым узел пускает в дверь управления. Без него узел
// отдаёт сайт-приманку вместо ответа, и разбор JSON спотыкается об HTML.
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
