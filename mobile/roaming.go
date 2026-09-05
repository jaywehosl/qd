package qdmobile

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
)

var moving atomic.Bool

// watch держит туннель на месте, когда телефон меняет сеть. Здесь не нужен свой
// сторож за адресами: систему об этом спрашивать не надо, она сама говорит —
// NetworkChanged приходит от VpnService. Остаётся то, чего система не скажет:
// путь может умереть молча.
//
// Молчание пути нельзя мерить одними счётчиками. Телефон морозит процесс, пока
// он заморожен — не идёт ни keepalive, ни трафик, и узел снимает сессию по
// своему idle. Проснувшись, клиент видит ровный туннель: писать в него некому,
// значит и «глухоты» не видно, а QUIC о смерти не сообщает — наш собственный
// keepalive держит его idle-таймер живым и после того, как с той стороны никого
// не осталось. Такой туннель висит бесконечно, и приложения висят вместе с ним.
//
// Поэтому узел спрашивают напрямую — тем же запросом, каким клиент здоровается
// при дозвоне: ответил, значит путь жив и сессию помнят.
func (c *Client) watch(ctx context.Context, stop <-chan struct{}) {
	tick := time.NewTicker(deafStep)
	defer tick.Stop()

	c.mu.Lock()
	live := c.live
	c.mu.Unlock()
	if live == nil {
		return
	}

	was := live.Stats()
	deaf := time.Time{}
	heardAt := time.Now()
	last := time.Now()

	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-tick.C:
		}

		stood := time.Since(last)
		last = time.Now()

		if !live.Alive() {
			say("roam: the session is gone, coming back")
			go c.lost()
			return
		}

		// Процесс стоял: между двумя тиками прошло много больше, чем шаг. Пока он
		// стоял, узел успел снять сессию по своему idle, а счётчики этого не
		// покажут — в туннель было некому писать.
		if stood > goneFor {
			say("roam: the process stood still for %s, the node has dropped the session by now", stood.Round(time.Second))
			go c.lost()
			return
		}

		if stood > frozenFor {
			say("roam: the process stood still for %s, asking the path", stood.Round(time.Second))
			if !c.pathAnswers(ctx) {
				go c.lost()
				return
			}
			was, deaf, heardAt = live.Stats(), time.Time{}, time.Now()
			continue
		}

		now := live.Stats()
		heard := now.In != was.In
		spoke := now.Out != was.Out
		if now.Back != was.Back {
			heardAt = time.Now()
		}
		was = now

		if heard {
			if !deaf.IsZero() {
				say("roam: the path answers again")
			}
			deaf = time.Time{}
			continue
		}

		// Обратно давно ничего не приходило. Само по себе это не поломка — в
		// молчащий туннель никто не писал, — но и здоровьем это не считается:
		// мёртвый путь молчит точно так же. Спрашиваем сами, пока узел ещё
		// держит сессию.
		if time.Since(heardAt) > silenceFor {
			say("roam: nothing has come back for %s, asking the path", silenceFor)
			if !c.pathAnswers(ctx) {
				go c.lost()
				return
			}
			was, deaf, heardAt = live.Stats(), time.Time{}, time.Now()
			continue
		}

		if !spoke {
			deaf = time.Time{}
			continue
		}

		// Наружу пишем, обратно тишина — это уже глухота.
		if deaf.IsZero() {
			deaf = time.Now()
			continue
		}
		if time.Since(deaf) < deafFor {
			continue
		}
		if time.Since(deaf) < deafFor+patience {
			say("roam: nothing comes back for %s, trying to migrate in place", deafFor)
			c.migrate(ctx)
			deaf = time.Now().Add(-deafFor)
			continue
		}

		say("roam: the node stopped answering for %s, giving up on this path", patience)
		go c.lost()
		return
	}
}

// pathAnswers спрашивает узел по тому же соединению. Ответил — путь жив и
// сессию помнят; промолчал — ждать нечего, поднимаемся заново.
func (c *Client) pathAnswers(ctx context.Context) bool {
	c.mu.Lock()
	live := c.live
	c.mu.Unlock()
	if live == nil {
		return false
	}

	round, done := context.WithTimeout(ctx, askWait)
	err := live.Ask(round)
	done()
	if err == nil {
		return true
	}
	say("roam: the path does not answer: %v", err)
	return false
}

// migrate просит QUIC переехать на новый путь. Сессия на узле остаётся той же:
// ни нового рукопожатия, ни разрыва живых соединений в приложениях.
func (c *Client) migrate(ctx context.Context) {
	if !moving.CompareAndSwap(false, true) {
		return
	}
	defer moving.Store(false)

	c.mu.Lock()
	live := c.live
	c.mu.Unlock()
	if live == nil {
		return
	}

	for try := 1; try <= tries; try++ {
		round, done := context.WithTimeout(ctx, moveWait)
		err := live.Rebind(round)
		done()

		if err == nil {
			say("roam: the path moved, the tunnel migrated in place")
			return
		}
		if ctx.Err() != nil {
			return
		}
		say("roam: migration attempt %d of %d failed: %v", try, tries, err)

		select {
		case <-ctx.Done():
			return
		case <-time.After(pause):
		}
	}

	say("roam: the path did not move, coming back through a fresh dial")
	go c.lost()
}

var resolved sync.Map

func lookUp(host string) []netip.Addr {
	ctx, stop := context.WithTimeout(context.Background(), lookWait)
	defer stop()

	found, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		if held, known := resolved.Load(host); known {
			return held.([]netip.Addr)
		}
		say("bypass: could not resolve %s: %v", host, err)
		return nil
	}

	out := make([]netip.Addr, 0, len(found))
	for _, addr := range found {
		out = append(out, addr.Unmap())
	}
	resolved.Store(host, out)
	return out
}

const (
	deafStep   = 3 * time.Second
	deafFor    = 20 * time.Second
	patience   = 45 * time.Second
	frozenFor  = 30 * time.Second
	goneFor    = 75 * time.Second
	silenceFor = 60 * time.Second
	askWait    = 3 * time.Second
	tries      = 2
	moveWait   = 4 * time.Second
	pause      = 1 * time.Second
	lookWait   = 3 * time.Second
)
