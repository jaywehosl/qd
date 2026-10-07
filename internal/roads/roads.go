package roads

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/qd/internal/qsrv/uplink/quicconn"
)

const quicHeadStart = time.Second

const nextAddress = 250 * time.Millisecond

var (
	only      atomic.Bool
	seen      sync.Map
	relayMode atomic.Bool
	pinned    atomic.Pointer[Path]
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

func Follow(p Path) (release func()) {
	held := &p
	pinned.Store(held)
	return func() { pinned.CompareAndSwap(held, nil) }
}

func Following(endpoint string) (Path, bool) {
	held := pinned.Load()
	if held == nil || held.Endpoint != endpoint {
		return Path{}, false
	}
	return *held, true
}

func Only(on bool) { only.Store(on) }

func OnlyTCP() bool { return only.Load() }

func Remember(endpoint string, overTCP bool) { seen.Store(endpoint, overTCP) }

func SetRelay(on bool) { relayMode.Store(on) }

func RelayMode() bool { return relayMode.Load() }

func Forget() {
	seen.Range(func(k, _ any) bool {
		seen.Delete(k)
		return true
	})
	relayMode.Store(false)
}

func HeadStart(endpoint string) time.Duration {
	if only.Load() {
		return 0
	}
	if held, ok := seen.Load(endpoint); ok && held.(bool) {
		return 0
	}
	return quicHeadStart
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
				case <-time.After(time.Duration(n) * nextAddress):
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
