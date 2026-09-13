package roads

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink/quicconn"
)

const quicHeadStart = 300 * time.Millisecond

var (
	only      atomic.Bool
	seen      sync.Map
	relayMode atomic.Bool
)

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

	var last error
	for _, where := range addrs {
		conn, err := dialer.DialContext(ctx, "tcp", where)
		if err == nil {
			return conn, nil
		}
		last = err
		if ctx.Err() != nil {
			break
		}
	}
	if last == nil {
		last = fmt.Errorf("no address for %s", endpoint)
	}
	return nil, last
}
