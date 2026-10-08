package clientrun

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/qd/internal/pace"
	"github.com/jaywehosl/qd/internal/qcli"
	"github.com/jaywehosl/qd/internal/roads"
)

var ErrBetter = errors.New("a better road answers")

var (
	lastSwitch atomic.Pointer[time.Time]
	switches   atomic.Int32
)

func switchPause() time.Duration { return pace.SwitchRest << min(switches.Load(), 5) }

type Watch struct {
	Live    *qcli.Tunnel
	Changed <-chan struct{}
	Migrate func(ctx context.Context) bool
	Lost    func(why error)
	Say     func(format string, args ...any)
}

func (w Watch) Run(ctx context.Context, stop <-chan struct{}) {
	live := w.Live
	tick := time.NewTicker(pace.WatchStep)
	defer tick.Stop()

	was := live.Stats()
	deaf := time.Time{}
	heardAt := time.Now()
	last := time.Now()
	better := live.Better()
	var later <-chan time.Time
	if live.CanMigrate() {
		switches.Store(0)
	}

	comeOver := func() {
		now := time.Now()
		lastSwitch.Store(&now)
		switches.Add(1)
		w.Say("roam: a better road answers, coming back over it")
		w.Lost(ErrBetter)
	}

	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return

		case <-better:
			better = nil
			if held := lastSwitch.Load(); held != nil && time.Since(*held) < switchPause() {
				later = time.After(switchPause() - time.Since(*held))
				continue
			}
			comeOver()
			return

		case <-later:
			comeOver()
			return

		case <-w.Changed:
			roads.Forget()
			was, deaf, heardAt, last = live.Stats(), time.Time{}, time.Now(), time.Now()
			if !live.CanMigrate() {
				if !PathAnswers(ctx, live, pace.AskWait, w.Say) {
					if ctx.Err() == nil {
						w.Lost(fmt.Errorf("the network changed and the path does not answer"))
					}
					return
				}
				live.Reseek()
				continue
			}
			if !w.Migrate(ctx) {
				if ctx.Err() == nil {
					w.Lost(fmt.Errorf("the network changed and the path did not move"))
				}
				return
			}

		case <-tick.C:
			select {
			case <-stop:
				return
			default:
			}
			if ctx.Err() != nil {
				return
			}
			stood := time.Since(last)
			last = time.Now()

			if !live.Alive() {
				w.Lost(fmt.Errorf("the session is gone"))
				return
			}
			if stood > pace.Gone {
				w.Lost(fmt.Errorf("the process stood still for %s, the node has dropped the session by now", stood.Round(time.Second)))
				return
			}
			if stood > pace.Frozen {
				w.Say("roam: the process stood still for %s, asking the path", stood.Round(time.Second))
				if !PathAnswers(ctx, live, pace.AskWait, w.Say) {
					w.Lost(fmt.Errorf("the path did not answer after a %s pause", stood.Round(time.Second)))
					return
				}
				was, deaf, heardAt = live.Stats(), time.Time{}, time.Now()
				continue
			}

			now := live.Stats()
			heard := now.Heard != was.Heard
			spoke := now.Out != was.Out
			if now.Back != was.Back {
				heardAt = time.Now()
			}
			was = now

			if heard {
				if !deaf.IsZero() {
					w.Say("roam: the path answers again")
				}
				deaf = time.Time{}
				continue
			}
			if time.Since(heardAt) > pace.Silence {
				w.Say("roam: nothing has come back for %s, asking the path", pace.Silence)
				if !PathAnswers(ctx, live, pace.AskWait, w.Say) {
					w.Lost(fmt.Errorf("the path stopped answering while idle"))
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
			if time.Since(deaf) < pace.Deaf {
				continue
			}
			if time.Since(deaf) < pace.Deaf+pace.Patience {
				w.Say("roam: nothing comes back for %s, trying to migrate in place", pace.Deaf)
				if !w.Migrate(ctx) {
					if ctx.Err() == nil {
						w.Lost(fmt.Errorf("the node stopped answering and the path did not move"))
					}
					return
				}
				deaf = time.Now().Add(-pace.Deaf)
				continue
			}
			w.Lost(fmt.Errorf("the node stopped answering for %s and migration did not help", pace.Patience))
			return
		}
	}
}

func PathAnswers(ctx context.Context, live *qcli.Tunnel, wait time.Duration, say func(string, ...any)) bool {
	round, done := context.WithTimeout(ctx, wait)
	err := live.Ask(round)
	done()
	if err == nil {
		return true
	}
	say("roam: the path does not answer: %v", err)
	return false
}

func Move(ctx context.Context, live *qcli.Tunnel, say func(string, ...any)) bool {
	if !live.CanMigrate() {
		say("roam: this path does not migrate, bringing the tunnel up again")
		return false
	}
	for try := 1; ; try++ {
		round, done := context.WithTimeout(ctx, pace.MoveWait)
		err := live.Rebind(round)
		done()
		if err == nil {
			say("roam: the path moved, the tunnel migrated in place")
			return true
		}
		if ctx.Err() != nil {
			return false
		}
		say("roam: migration attempt %d of %d failed: %v", try, pace.MoveTries, err)
		if try >= pace.MoveTries {
			break
		}

		select {
		case <-ctx.Done():
			return false
		case <-time.After(pace.MovePause):
		}
	}
	say("roam: the path did not move, coming back through a fresh dial")
	return false
}

func Prove(live *qcli.Tunnel, held func() bool, say func(string, ...any)) bool {
	for _, wait := range []time.Duration{time.Second, 3 * time.Second, 4 * time.Second} {
		time.Sleep(wait)
		if !held() {
			return true
		}
		if !PathAnswers(context.Background(), live, pace.ProveWait, say) {
			say("roam: the moved path went quiet, coming back through a fresh dial")
			return false
		}
	}
	return true
}
