package roads

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/qd/internal/pace"
	"github.com/jaywehosl/qd/internal/qsrv/uplink/quicconn"
)

type Rung int

const (
	QUIC Rung = iota
	TCP
	Relay
)

var (
	only atomic.Bool
	held sync.Map
)

type Path struct {
	Endpoint string
	OverTCP  bool
	Relay    string
	Hidden   bool
}

func (p Path) String() string {
	switch {
	case p.Relay != "":
		return p.Endpoint + " over relay " + p.Relay
	case p.OverTCP:
		return p.Endpoint + " over tcp"
	default:
		return p.Endpoint + " over quic"
	}
}

func (p Path) Short() string {
	switch {
	case p.Relay != "":
		return "R+H3"
	case p.OverTCP:
		return "H2"
	default:
		return "H3"
	}
}

func Only(on bool) { only.Store(on) }

func OnlyTCP() bool { return only.Load() }

func Remember(endpoint string, rung Rung) { held.Store(endpoint, rung) }

func Recall(endpoint string) (Rung, bool) {
	was, ok := held.Load(endpoint)
	if !ok {
		return QUIC, false
	}
	return was.(Rung), true
}

func Forget() {
	held.Clear()
	quicconn.Forget()
}

type Try[T any] struct {
	Rung Rung
	Run  func(ctx context.Context) (T, error)
}

func due(rung, known Rung, remembered bool) time.Duration {
	switch {
	case rung == QUIC, remembered && known >= rung:
		return 0
	case rung == TCP:
		return pace.QUICHeadStart
	default:
		return pace.RelayStart
	}
}

func Climb[T any](ctx context.Context, endpoint string, tries []Try[T], drop func(T), better func()) (T, Rung, error) {
	var none T
	if len(tries) == 0 {
		return none, QUIC, errors.New("no road to try")
	}
	known, remembered := Recall(endpoint)

	all, cancelAll := context.WithCancel(context.WithoutCancel(ctx))
	detach := context.AfterFunc(ctx, cancelAll)

	type finish struct {
		n    int
		road T
		err  error
	}
	line := make(chan finish, len(tries))
	stops := make([]context.CancelFunc, len(tries))
	begun := make([]bool, len(tries))
	over := make([]bool, len(tries))

	running := func() int {
		left := 0
		for n := range tries {
			if begun[n] && !over[n] {
				left++
			}
		}
		return left
	}
	aboveEnded := func(rung Rung) bool {
		for n, t := range tries {
			if t.Rung < rung && !over[n] {
				return false
			}
		}
		return true
	}

	began := time.Now()
	launch := func() time.Duration {
		next := time.Duration(-1)
		for n, t := range tries {
			if begun[n] {
				continue
			}
			wait := time.Until(began.Add(due(t.Rung, known, remembered)))
			if wait > 0 && !aboveEnded(t.Rung) {
				if next < 0 || wait < next {
					next = wait
				}
				continue
			}
			round, stop := context.WithCancel(all)
			stops[n], begun[n] = stop, true
			go func(n int, run func(context.Context) (T, error)) {
				road, err := run(round)
				line <- finish{n, road, err}
			}(n, t.Run)
		}
		return next
	}

	tick := time.NewTimer(time.Hour)
	defer tick.Stop()
	var refused []string
	for left := len(tries); left > 0; {
		if next := launch(); next >= 0 {
			tick.Reset(next)
		}
		select {
		case <-tick.C:
		case <-ctx.Done():
			detach()
			cancelAll()
			go func(pending int) {
				for ; pending > 0; pending-- {
					if late := <-line; late.err == nil {
						drop(late.road)
					}
				}
			}(running())
			return none, QUIC, ctx.Err()
		case got := <-line:
			over[got.n] = true
			left--
			if got.err != nil {
				refused = append(refused, got.err.Error())
				continue
			}

			won := tries[got.n].Rung
			detach()
			for n, t := range tries {
				if begun[n] && !over[n] && t.Rung >= won {
					stops[n]()
				}
			}
			Remember(endpoint, won)
			pending := running()
			if pending == 0 {
				cancelAll()
				return got.road, won, nil
			}
			go func() {
				until := time.NewTimer(time.Until(began.Add(pace.LateFor)))
				defer until.Stop()
				climbed := false
				for pending > 0 {
					select {
					case late := <-line:
						pending--
						if late.err != nil {
							continue
						}
						drop(late.road)
						if rung := tries[late.n].Rung; rung < won && !climbed {
							climbed = true
							Remember(endpoint, rung)
							if better != nil {
								better()
							}
						}
					case <-until.C:
						cancelAll()
					}
				}
				cancelAll()
			}()
			return got.road, won, nil
		}
	}
	detach()
	cancelAll()
	return none, QUIC, errors.New(strings.Join(refused, " / "))
}

func ReachTCP(ctx context.Context, dialer *net.Dialer, endpoint string) (net.Conn, error) {
	if dialer == nil {
		dialer = &net.Dialer{}
	}
	addrs, err := quicconn.Addrs(ctx, endpoint)
	if err != nil {
		return dialer.DialContext(ctx, "tcp", endpoint)
	}

	round, stop := context.WithCancel(ctx)
	defer stop()

	type finish struct {
		conn net.Conn
		err  error
	}
	line := make(chan finish, len(addrs))
	for i, where := range addrs {
		go func(n int, where string) {
			if n > 0 {
				select {
				case <-time.After(time.Duration(n) * pace.Stagger):
				case <-round.Done():
					line <- finish{err: round.Err()}
					return
				}
			}
			conn, err := dialer.DialContext(round, "tcp", where)
			line <- finish{conn, err}
		}(i, where)
	}

	var last error
	for i := range addrs {
		got := <-line
		if got.err == nil {
			go func(left int) {
				for ; left > 0; left-- {
					if late := <-line; late.conn != nil {
						late.conn.Close()
					}
				}
			}(len(addrs) - i - 1)
			return got.conn, nil
		}
		last = got.err
	}
	if last == nil {
		last = fmt.Errorf("no address for %s", endpoint)
	}
	return nil, last
}
