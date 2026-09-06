// Package roads — общая память о том, каким путём узел отвечает: датаграммами
// поверх QUIC или стримами поверх TCP.
//
// Память одна на клиента намеренно. Туннель и управляющий канал ходят к одному
// и тому же узлу; когда каждый вёл свой список, на сети без UDP оба честно
// ждали свою фору, и цена блокировки платилась дважды.
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

// QUICHeadStart — фора датаграммному пути. Он лучше, поэтому TCP выходит на
// дистанцию только если за это время QUIC не ответил.
const QUICHeadStart = 300 * time.Millisecond

var (
	only atomic.Bool
	seen sync.Map
)

// Only заставляет ходить только стримами. Нужно для отладки: обычно путь
// выбирается гонкой.
func Only(on bool) { only.Store(on) }

func OnlyTCP() bool { return only.Load() }

func Remember(endpoint string, overTCP bool) { seen.Store(endpoint, overTCP) }

// Forget сбрасывает память о путях: после смены сети прежний ответ ничего не
// значит, UDP мог и открыться, и закрыться.
func Forget() {
	seen.Range(func(k, _ any) bool {
		seen.Delete(k)
		return true
	})
}

// HeadStart — сколько ждать перед попыткой по TCP. Ноль, если этот узел уже
// отвечал стримами: второй раз ждать впустую незачем.
func HeadStart(endpoint string) time.Duration {
	if only.Load() {
		return 0
	}
	if held, ok := seen.Load(endpoint); ok && held.(bool) {
		return 0
	}
	return QUICHeadStart
}

// ReachTCP дозванивается по всем адресам имени в том порядке, в каком их даёт
// quicconn: сперва то семейство, до которого у машины есть путь.
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
