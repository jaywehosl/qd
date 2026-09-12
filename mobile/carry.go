package qdmobile

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"time"

	"github.com/jaywehosl/quic-diver/internal/clientdns"
	"github.com/jaywehosl/quic-diver/internal/clientrun"
	"github.com/jaywehosl/quic-diver/internal/qcli"
	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
	"github.com/jaywehosl/quic-diver/internal/qcli/packet/tun"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
	"github.com/jaywehosl/quic-diver/internal/qsrv"
)

// carry поднимает туннель тем же порядком, что и клиент под Windows: сам
// порядок живёт в clientrun, здесь остаётся только то, чем телефон отличается —
// устройство от VpnService вместо драйвера захвата.
func (c *Client) carry(servers []string, relays []qcli.RelayLink, session uint32) error {
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

	// Прежний движок валим безусловно, даже если считаем, что его нет. Иначе
	// осиротевший продолжает читать устройство и глотать трафик: значок VPN
	// горит, а связи нет.
	c.mu.Lock()
	was := c.liveStop
	c.liveStop = nil
	c.mu.Unlock()
	if was != nil {
		say("carry: an older data path was still around, dropping it")
		was()
	}

	seen := c.seen
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
			Fast:      runFast,
			Bypass:    c.keepOut(),
			Keep:      c.keeper(),
			Exit:      c.exitFor,
			Direct:    c.goesDirect,
			Loud:      loud.Load(),
		},
		Wait: dialWait,
		DNS: &clientdns.Config{
			Node: servers[0], Token: c.token(), Ask: c.wire().Ask,
			Say: say, Keep: recentKept,
			Blocked: func(name string) bool { return seen != nil && seen.Query(name) },
		},
		// Устройство поднимаем под выданный адрес, а не наугад: узел сверяет, с
		// какого адреса пришла датаграмма, и чужой отбрасывает.
		Source: func(ctx context.Context, live *qcli.Tunnel) (packet.Source, error) {
			fd, err := c.hold(live.Assigned()[0], mtu)
			if err != nil {
				return nil, err
			}
			raw, err := tun.Open(fd, mtu)
			if err != nil {
				return nil, err
			}
			return watched(raw), nil
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
	c.dns, c.server, c.gone = held.DNS, held.Endpoint, held.Gone
	c.src = held.Source
	c.mu.Unlock()

	say("carry: datagram limit %d with mtu %d", held.Live.DatagramLimit(), mtu)

	go c.announce("join")
	go c.watch(held.Ctx, held.Halt)

	return nil
}

func (c *Client) stopCarry() {
	say("carry: stop asked by %s", whoCalled())

	// Дозвон рвём до захвата turn: иначе ждали бы его конца, а он длится до
	// двадцати секунд, и все нажатия это время уходили бы в пустоту.
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
	// Устройство гасим первым: пока оно открыто, датапуть может висеть на нём
	// и не заметить отмены.
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

	// Ждём конца датапути в стороне: держать на этом замок перехода значит
	// заставить следующее нажатие ждать три секунды впустую.
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

// lost поднимает туннель заново, когда датапуть больше не жив. Миграцию к этому
// моменту уже пробовал сторож.
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

// exitFor решает судьбу одного флоу: общий выход, если правил по приложениям нет,
// иначе — то, что сказано про хозяина этого флоу. Для соединения хозяина ждём,
// для датаграмм не ждём: они идут потоком, и остановка на опрос системы стоила
// бы дороже, чем первые пакеты общим выходом.
func (c *Client) exitFor(src, dst netip.AddrPort, udp bool) string {
	if c.marks.forFlow(src, dst, udp, !udp) == qdcrypt.ExitEgress {
		return qsrv.AnyExit
	}
	return ""
}

// goesDirect отвечает на вопрос «пустить мимо туннеля вообще». Метки приложений
// говорят о другом — каким выходом идти, — и путать это с обходом нельзя: при
// выключенном +egress весь трафик считался прямым и туннель стоял пустым.
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

const recentKept = 64
