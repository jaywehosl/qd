//go:build windows

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jaywehosl/quic-diver/internal/qcli"
	"github.com/jaywehosl/quic-diver/internal/roam"
)

// roamWatch держит туннель на месте при смене сети. Адрес поменялся — QUIC
// переезжает миграцией по connection ID, соединение остаётся тем же: ни нового
// рукопожатия, ни новой сессии на узле, ни разрыва живых флоу.
//
// Переподнимать туннель здесь нельзя: это была бы гонка входов из-под ног у
// работающих соединений.
//
// Мёртвый путь одними счётчиками не отличить от простаивающего. Машина уходит в
// сон — процесс стоит, keepalive не идёт, и узел снимает сессию по своему idle.
// Проснувшись, клиент видит ровный туннель: писать в него некому, «глухоты» не
// видно, а QUIC о смерти не сообщает — наш же keepalive держит его idle-таймер
// живым и после того, как с той стороны никого не осталось. Поэтому узел
// спрашивают напрямую — тем же запросом, каким клиент здоровается при дозвоне:
// ответил, значит путь жив и сессию помнят.
func roamWatch(ctx context.Context, stop <-chan struct{}, live *qcli.Tunnel, lost func(error)) {
	defer func() {
		if fell := recover(); fell != nil {
			fmt.Printf("roam     watch gave up: %v\n", fell)
		}
	}()

	watcher, err := roam.NewSystemWatcher()
	if err != nil {
		fmt.Printf("roam     no watch on this machine: %v\n", err)
		return
	}
	defer watcher.Close()

	tick := time.NewTicker(roamStep)
	defer tick.Stop()

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

		case <-watcher.Changed():
			was, deaf, heardAt, last = live.Stats(), time.Time{}, time.Now(), time.Now()
			migrate(ctx, live)

		case <-tick.C:
			stood := time.Since(last)
			last = time.Now()

			if !live.Alive() {
				lost(fmt.Errorf("the session is gone"))
				return
			}

			// Процесс стоял (сон машины, заморозка): пока он стоял, узел успел
			// снять сессию по своему idle, а счётчики этого не покажут — в
			// туннель было некому писать.
			if stood > goneFor {
				lost(fmt.Errorf("the process stood still for %s, the node has dropped the session by now", stood.Round(time.Second)))
				return
			}

			if stood > frozenFor {
				fmt.Printf("roam     the process stood still for %s, asking the path\n", stood.Round(time.Second))
				if !pathAnswers(ctx, live) {
					lost(fmt.Errorf("the path did not answer after a %s pause", stood.Round(time.Second)))
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
					fmt.Printf("roam     the path answers again\n")
				}
				deaf = time.Time{}
				continue
			}

			// Обратно давно ничего не приходило. Само по себе это не поломка — в
			// молчащий туннель никто не писал, — но и здоровьем это не считается:
			// мёртвый путь молчит так же. Спрашиваем сами, пока узел ещё держит
			// сессию.
			if time.Since(heardAt) > silenceFor {
				fmt.Printf("roam     nothing has come back for %s, asking the path\n", silenceFor)
				if !pathAnswers(ctx, live) {
					lost(fmt.Errorf("the path stopped answering while idle"))
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
			if time.Since(deaf) < deafFor+roamPatience {
				fmt.Printf("roam     nothing comes back, trying to migrate in place\n")
				migrate(ctx, live)
				deaf = time.Now().Add(-deafFor)
				continue
			}
			lost(fmt.Errorf("the node stopped answering for %s and migration did not help", roamPatience))
			return
		}
	}
}

// pathAnswers спрашивает узел по тому же соединению и ждёт ответа.
func pathAnswers(ctx context.Context, live *qcli.Tunnel) bool {
	round, done := context.WithTimeout(ctx, askWait)
	err := live.Ask(round)
	done()
	if err == nil {
		return true
	}
	fmt.Printf("roam     the path does not answer: %v\n", err)
	return false
}

// migrate просит QUIC переехать на новый путь. Смена сети редко бывает мгновенной:
// адрес уже другой, а маршрут или DHCP ещё нет, поэтому одной попытки мало.
func migrate(ctx context.Context, live *qcli.Tunnel) {
	for try := 1; try <= roamTries; try++ {
		round, done := context.WithTimeout(ctx, roamWait)
		err := live.Rebind(round)
		done()

		if err == nil {
			fmt.Printf("roam     the path moved, the tunnel migrated in place\n")
			return
		}
		if ctx.Err() != nil {
			return
		}
		fmt.Printf("roam     migration attempt %d of %d failed: %v\n", try, roamTries, err)

		select {
		case <-ctx.Done():
			return
		case <-time.After(roamPause):
		}
	}
}

const (
	roamStep     = 3 * time.Second
	roamWait     = 8 * time.Second
	roamPause    = 2 * time.Second
	roamTries    = 3
	deafFor      = 20 * time.Second
	roamPatience = 45 * time.Second
	frozenFor    = 30 * time.Second
	goneFor      = 75 * time.Second
	silenceFor   = 60 * time.Second
	askWait      = 3 * time.Second
)
