package qdmobile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/jaywehosl/quic-diver/internal/qcli"
	"github.com/jaywehosl/quic-diver/internal/qcli/packet/tun"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
	"github.com/jaywehosl/quic-diver/internal/qsrv"
)

func (c *Client) carry(servers []string, session uint32) error {
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

	dns, err := newResolver(servers[0], c.token(), c.seen, c.wire().Ask)
	if err != nil {
		return fmt.Errorf("dns: %w", err)
	}

	began := time.Now()
	ctx, cancel := context.WithCancel(context.Background())

	dialCtx, dialStop := context.WithTimeout(ctx, dialWait)
	live, err := qcli.Dial(dialCtx, qcli.Options{
		Endpoints: servers,
		Token:     c.token(),
		Route:     c.route(),
		MTU:       mtu,
		Workers:   readers,
		Brutal:    int(c.rate.Load()),
		Fast:      runFast,
		Resolver:  dns.Addr(),
		Bypass:    c.keepOut(),
		Keep:      c.keeper(),
		Exit:      c.exitFor,
		Direct:    c.goesDirect,
		Loud:      loud.Load(),
	})
	dialStop()
	if err != nil {
		say("carry: could not reach any entrypoint: %v", err)
		cancel()
		dns.Close()
		return err
	}

	assigned := live.Assigned()
	if len(assigned) == 0 {
		cancel()
		live.Close()
		dns.Close()
		return errors.New("the node assigned no address")
	}

	dns.SetNode(live.Endpoint())

	fd, err := c.hold(assigned[0], mtu)
	if err != nil {
		cancel()
		live.Close()
		dns.Close()
		return err
	}

	raw, err := tun.Open(fd, mtu)
	if err != nil {
		cancel()
		live.Close()
		dns.Close()
		return err
	}
	src := watched(raw)

	stop := make(chan struct{})

	c.mu.Lock()
	c.stop, c.live, c.liveStop, c.session, c.running = stop, live, cancel, session, true
	c.dns, c.server = dns, live.Endpoint()
	c.mu.Unlock()

	say("carry: up in %d ms through %s, node gave %s, dns on %s",
		time.Since(began).Milliseconds(), live.Endpoint(), assigned[0], dns.Addr())
	say("carry: datagram limit %d with mtu %d", live.DatagramLimit(), mtu)

	gone := make(chan struct{})
	c.mu.Lock()
	c.gone = gone
	c.mu.Unlock()

	go func() {
		defer close(gone)
		defer src.Close()
		err := live.Run(ctx, src)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = errors.New("the data path stopped")
		}
		say("carry: stopped: %v", err)
		go c.lost()
	}()

	go dns.Serve(stop)
	go dns.keepWarm(stop)
	go c.announce("join")
	go c.watch(ctx, stop)

	return nil
}

func (c *Client) stopCarry() {
	say("carry: stop asked by %s", whoCalled())

	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	stop, live, cancel, dns, gone := c.stop, c.live, c.liveStop, c.dns, c.gone
	c.running = false
	c.stop, c.live, c.liveStop, c.dns = nil, nil, nil, nil
	c.mu.Unlock()

	go c.announce("bye")

	close(stop)
	if cancel != nil {
		cancel()
	}
	dns.Close()
	if live == nil {
		return
	}

	if gone != nil {
		select {
		case <-gone:
		case <-time.After(stopWait):
			say("carry: the data path did not stop in time")
		}
	}

	go live.Close()
}

func (c *Client) lost() {
	c.stopCarry()

	time.Sleep(settle)

	sub, err := c.db.Subscription()
	if err != nil || !sub.Imported {
		return
	}
	if err := c.api.Connect(); err != nil {
		say("carry: could not come back: %v", err)
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
	if c.marks.forFlow(src, dst, udp, !udp) == qdcrypt.ExitEgress {
		return qsrv.AnyExit
	}
	return ""
}

func (c *Client) goesDirect(pkt []byte) bool { return false }

func (c *Client) keepOut() []netip.Prefix {
	out := []netip.Prefix{}
	for _, host := range c.peerAddresses() {
		if addr, err := netip.ParseAddr(host); err == nil {
			out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
			continue
		}
		for _, addr := range lookUp(host) {
			out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
		}
	}
	return out
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

var _ = fmt.Sprint
