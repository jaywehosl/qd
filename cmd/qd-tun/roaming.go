//go:build windows

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jaywehosl/quic-diver/internal/qcli"
	"github.com/jaywehosl/quic-diver/internal/roam"
)

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
