//go:build windows || linux

package main

import (
	"context"
	"fmt"

	"github.com/jaywehosl/qd/internal/clientrun"
	"github.com/jaywehosl/qd/internal/qcli"
)

var errBetter = clientrun.ErrBetter

func roamSay(format string, args ...any) { fmt.Printf(format+"\n", args...) }

func roamWatch(ctx context.Context, stop <-chan struct{}, live *qcli.Tunnel, lost func(error)) {
	defer func() {
		if fell := recover(); fell != nil {
			fmt.Printf("roam     watch gave up: %v\n", fell)
		}
	}()

	var changed <-chan struct{}
	if watcher, err := watchRoutes(); err != nil {
		fmt.Printf("roam     no route watch on this machine, only the liveness checks run: %v\n", err)
	} else {
		defer watcher.Close()
		changed = watcher.Changed()
	}

	held := func() bool {
		now := liveTunnel.Load()
		return now != nil && *now == live
	}
	clientrun.Watch{
		Live:    live,
		Changed: changed,
		Migrate: func(ctx context.Context) bool {
			if !clientrun.Move(ctx, live, roamSay) {
				return false
			}
			nodeTalk.Reset()
			go func() {
				if !clientrun.Prove(live, held, roamSay) && held() {
					lost(fmt.Errorf("the moved path went quiet"))
				}
			}()
			return true
		},
		Lost: lost,
		Say:  roamSay,
	}.Run(ctx, stop)
}
