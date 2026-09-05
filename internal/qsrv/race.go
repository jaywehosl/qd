package qsrv

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/quic-go/quic-go/http3"
)

func (n *Node) raceExit(ctx context.Context, route string, seat uint32) (*http3.ClientConn, string, error) {
	runners := n.exitsFor(route)
	if len(runners) == 0 {
		return nil, "", fmt.Errorf("no exit answers to %q", route)
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
			taken := false
			once.Do(func() { taken = true })
			if !taken {
				n.links.drop(p.Endpoint, seat)
				line <- finish{err: fmt.Errorf("%s lost the race", p.Endpoint)}
				return
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
