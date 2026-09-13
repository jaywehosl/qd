//go:build linux

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jaywehosl/quic-diver/internal/netstate"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
	"github.com/jaywehosl/quic-diver/internal/qsrv"
	"github.com/jaywehosl/quic-diver/internal/store"
)

type gate struct {
	mu      sync.RWMutex
	allowed map[uint32]bool
	network string
}

func newGate() *gate { return &gate{allowed: map[uint32]bool{}} }

func (g *gate) list() map[uint32]struct{} {
	g.mu.RLock()
	defer g.mu.RUnlock()

	out := make(map[uint32]struct{}, len(g.allowed))
	for id := range g.allowed {
		out[id] = struct{}{}
	}
	return out
}

func (g *gate) add(id uint32) {
	g.mu.Lock()
	if _, held := g.allowed[id]; !held {
		g.allowed[id] = false
	}
	g.mu.Unlock()
}

func (g *gate) del(id uint32) {
	g.mu.Lock()
	delete(g.allowed, id)
	g.mu.Unlock()
}

func (g *gate) exit(id uint32, allow bool) {
	g.mu.Lock()
	if _, held := g.allowed[id]; held {
		g.allowed[id] = allow
	}
	g.mu.Unlock()
}

func (g *gate) setNetwork(key string) {
	g.mu.Lock()
	g.network = key
	g.mu.Unlock()
}

func (g *gate) verify(raw string) (qsrv.Grant, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.network != "" && strings.EqualFold(raw, g.network) {
		return qsrv.Grant{Client: "network key", AllowExit: false, Session: 0}, true
	}

	id := qdcrypt.SessionID(raw)
	allowExit, held := g.allowed[id]
	if !held {
		return qsrv.Grant{}, false
	}
	return qsrv.Grant{Client: raw, AllowExit: allowExit, Session: id}, true
}

func tunablesFrom(s store.NetworkSettings) qsrv.Tunables {
	return qsrv.Tunables{
		MaxStreams:   int64(s.MaxStreams),
		StreamWindow: uint64(s.StreamWindow) << 10,
		MaxStreamWin: uint64(s.MaxStreamWindow) << 10,
		ConnWindow:   uint64(s.ConnWindow) << 10,
		MaxConnWin:   uint64(s.MaxConnWindow) << 10,
		IdleTimeout:  time.Duration(s.IdleSeconds) * time.Second,
		KeepAlive:    time.Duration(s.KeepAliveSeconds) * time.Second,
		SocketBuffer: s.SocketBuffer << 10,
		MTU:          s.MTU,
	}
}

func peersFrom(db *store.DB, selfID int) func() []qsrv.Peer {
	return func() []qsrv.Peer {
		nodes, err := db.Nodes()
		if err != nil {
			log.Printf("peers      the network database will not read: %v", err)
			return nil
		}

		out := []qsrv.Peer{}
		for _, n := range nodes {
			if n.ID == selfID || !n.Enable || n.Role != netstate.RoleEgress || n.Address == "" {
				continue
			}
			out = append(out, qsrv.Peer{
				ID:       n.UUID,
				Tag:      n.Tag,
				Endpoint: net.JoinHostPort(n.Address, strconv.Itoa(n.Port)),
			})
		}
		return out
	}
}

func loadTLS(certFile, keyFile, authority string) (*tls.Config, error) {
	if certFile == "" || keyFile == "" {
		host := authority
		if h, _, err := net.SplitHostPort(authority); err == nil {
			host = h
		}
		return qsrv.DevTLS(host)
	}
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{pair}}, nil
}

func poolOf(text string) netip.Prefix {
	p, _ := netip.ParsePrefix(text)
	return p
}

func runNode(ctx context.Context, node *qsrv.Node) {
	if err := node.Run(ctx); err != nil && ctx.Err() == nil {
		fmt.Printf("quic       stopped listening: %v\n", err)
	}
}
