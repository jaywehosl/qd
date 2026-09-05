package qsrv

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/quic-go/quic-go/http3"
)

// raceExit выбирает выход гонкой, но только когда выбирать действительно нужно.
// Живая связь с подходящим выходом — уже готовый ответ: гонять её заново значит
// на каждый флоу дозваниваться до всех выходов и рвать проигравшую связь, по
// которой в этот же миг едет соседний флоу.
//
// Метка может называть конкретный узел (по имени или uuid) — тогда гонки нет,
// есть один кандидат, и его недоступность означает отказ.
func (n *Node) raceExit(ctx context.Context, route string, seat uint32) (*http3.ClientConn, string, error) {
	known := n.peers()
	runners := n.exitsFor(route)
	if len(runners) == 0 {
		return nil, "", fmt.Errorf("no exit answers to %q, this node knows %d exits", route, len(known))
	}

	if cc, endpoint, ok := n.links.standing(runners, seat); ok {
		return cc, endpoint, nil
	}

	round, stop := context.WithTimeout(ctx, exitRace)
	defer stop()

	type finish struct {
		cc       *http3.ClientConn
		endpoint string
		err      error
	}
	line := make(chan finish, len(runners))

	var once sync.Once

	for _, peer := range runners {
		go func(p Peer) {
			cc, err := n.links.to(p.Endpoint, seat).connect(round)
			if err != nil {
				line <- finish{err: err}
				return
			}
			won := false
			once.Do(func() { won = true })
			if won {
				n.links.chose(seat, p.Endpoint)
			}
			line <- finish{cc: cc, endpoint: p.Endpoint}
		}(peer)
	}

	var last error
	for range runners {
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case got := <-line:
			if got.err != nil {
				last = got.err
				continue
			}
			return got.cc, got.endpoint, nil
		}
	}

	if last == nil {
		last = fmt.Errorf("no exit answered %q", route)
	}
	return nil, "", last
}

// exitsFor — соседи, которых метка допускает: назвали конкретного — он один,
// назвали категорию — все выходы сети.
func (n *Node) exitsFor(route string) []Peer {
	peers := n.peers()
	for _, p := range peers {
		if p.ID == route || p.Tag == route {
			return []Peer{p}
		}
	}
	if route != AnyExit {
		return nil
	}
	return peers
}

const exitRace = 8 * time.Second
