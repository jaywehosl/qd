package qdmobile

import (
	"context"
	"encoding/json"
	"net/netip"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/qd/internal/clientdns"
	"github.com/jaywehosl/qd/internal/clientrun"
	"github.com/jaywehosl/qd/internal/peers"
	"github.com/jaywehosl/qd/internal/qcli"
	"github.com/jaywehosl/qd/internal/qcli/packet"
	"github.com/jaywehosl/qd/internal/qcli/packet/tun"
	"github.com/jaywehosl/qd/internal/qdcrypt"
	"github.com/jaywehosl/qd/internal/qsrv"
	"github.com/jaywehosl/qd/internal/qsrv/uplink/relay"
)

func (c *Client) carry(servers []string, relays []relay.Link, session uint32) error {
	c.turn.Lock()
	defer c.turn.Unlock()

	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return nil
	}
	mtu := c.mtu
	c.mu.Unlock()
	if mtu <= 0 {
		mtu = safeMTU
	}
	say("carry: dialing %v with mtu %d", servers, mtu)

	c.mu.Lock()
	was := c.liveStop
	c.liveStop = nil
	c.mu.Unlock()
	if was != nil {
		say("carry: an older data path was still around, dropping it")
		was()
	}

	seen := c.seen
	carried := c.carriage()
	round, giveUp := context.WithCancel(context.Background())
	c.mu.Lock()
	c.dialing = giveUp
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.dialing = nil
		c.mu.Unlock()
	}()

	held, err := clientrun.Carry(round, clientrun.Plan{
		Dial: qcli.Options{
			Endpoints: servers,
			Relays:    relays,
			Token:     c.token(),
			Device:    c.device.ID,
			Route:     c.route(),
			MTU:       mtu,
			Workers:   readers,
			Brutal:    int(c.rate.Load()),
			BBR:       c.bbr(),
			Fast:      runFast,
			Bypass:    c.keepOut(),
			Keep:      c.keeper(),
			Exit:      c.exitFor,
			Direct:    c.goesDirect,
			Loud:      loud.Load(),
			Tickets:   c.db.Tickets(),
		},
		Wait: dialWait,
		DNS: &clientdns.Config{
			Node: servers[0], Token: c.token(), Ask: c.wire().Ask,
			Say:     say,
			Device:  c.device.ID,
			Exit:    func() bool { return exit.Load() == uint32(qdcrypt.ExitEgress) },
			Blocked: func(name string) bool { return seen != nil && seen.Query(name) },
		},
		Source: func(ctx context.Context, live *qcli.Tunnel) (packet.Source, error) {
			six, carried := live.Six()
			fd, err := c.hold(live.Assigned()[0], six, mtu)
			if err != nil {
				return nil, err
			}
			raw, err := tun.Open(fd)
			if err != nil {
				return nil, err
			}
			return watched(raw, carried), nil
		},
		Lost: func(error) { c.lost() },
		Say:  say,
	})
	if err != nil {
		say("carry: could not reach any entrypoint: %v", err)
		return err
	}

	c.mu.Lock()
	c.stop, c.live, c.liveStop, c.session, c.running = held.Halt, held.Live, held.Quit, session, true
	c.carried = carried
	c.pathTag = c.netTag
	orphaned.Store(false)
	c.dns, c.server, c.gone = held.DNS, held.Endpoint, held.Gone
	c.src = held.Source
	c.mu.Unlock()
	c.wanted.Store(true)

	say("carry: datagram limit %d with mtu %d", held.Live.DatagramLimit(), mtu)

	go c.announce("join")
	go c.watch(held.Ctx, held.Halt)

	return nil
}

func (c *Client) stopCarry() {
	say("carry: stop asked by %s", whoCalled())

	c.mu.Lock()
	giveUp := c.dialing
	c.mu.Unlock()
	if giveUp != nil {
		say("carry: stop during the dial, giving it up")
		giveUp()
	}

	c.turn.Lock()
	defer c.turn.Unlock()

	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	stop, live, cancel, dns, gone := c.stop, c.live, c.liveStop, c.dns, c.gone
	src := c.src
	c.running = false
	c.stop, c.live, c.liveStop, c.dns, c.src = nil, nil, nil, nil, nil
	c.mu.Unlock()

	go c.announce("bye")

	close(stop)
	if src != nil {
		src.Close()
	}
	if cancel != nil {
		cancel()
	}
	dns.Close()
	if live == nil {
		return
	}
	live.StopRelay()

	go func() {
		if gone != nil {
			select {
			case <-gone:
			case <-time.After(stopWait):
				say("carry: the data path did not stop in time")
			}
		}
		live.Close()
	}()
}

func (c *Client) lost() {
	c.stopCarry()
	c.comeBack()
}

var (
	backing atomic.Bool
	nudge   = make(chan struct{}, 1)
)

func (c *Client) comeBack() {
	if !backing.CompareAndSwap(false, true) {
		return
	}
	defer backing.Store(false)

	for pause := settle; ; pause = longer(pause) {
		select {
		case <-time.After(pause):
		case <-nudge:
		}
		if !c.wanted.Load() || c.Running() {
			return
		}
		sub, err := c.db.Subscription()
		if err != nil || !sub.Imported {
			return
		}
		err = c.api.Connect()
		if err == nil {
			return
		}
		say("carry: could not come back, next try in %s: %v", longer(pause), err)
	}
}

func longer(was time.Duration) time.Duration {
	switch {
	case was < 3*time.Second:
		return 3 * time.Second
	case was < 5*time.Second:
		return 5 * time.Second
	case was < 10*time.Second:
		return 10 * time.Second
	case was < 20*time.Second:
		return 20 * time.Second
	default:
		return 30 * time.Second
	}
}

func (c *Client) token() string {
	sub, err := c.db.Subscription()
	if err == nil && sub.Key != "" {
		return sub.Key
	}
	return ""
}

func (c *Client) route() string {
	if exit.Load() == uint32(qdcrypt.ExitEgress) {
		return qsrv.AnyExit
	}
	return ""
}

func (c *Client) exitFor(src, dst netip.AddrPort, udp bool) string {
	if c.marks.forFlow(src, dst, udp) == qdcrypt.ExitEgress {
		return qsrv.AnyExit
	}
	return ""
}

func (c *Client) goesDirect(pkt []byte) bool { return false }

func (c *Client) keepOut() []netip.Prefix {
	return peers.Prefixes(c.peerAddresses(), func(host string, err error) {
		say("bypass: could not resolve %s: %v", host, err)
	})
}

func (c *Client) StatsJSON() string {
	c.mu.Lock()
	live := c.live
	c.mu.Unlock()

	if live == nil {
		return "{}"
	}
	got := live.Stats()

	blob, err := json.Marshal(map[string]any{
		"packetsOut": got.Out,
		"packetsIn":  got.In,
		"bytesOut":   got.BytesOut,
		"bytesIn":    got.BytesIn,
	})
	if err != nil {
		return "{}"
	}
	return string(blob)
}

const (
	dialWait = 20 * time.Second
	stopWait = 3 * time.Second
)
