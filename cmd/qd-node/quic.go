//go:build linux

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jaywehosl/qd/internal/netstate"
	"github.com/jaywehosl/qd/internal/qdcrypt"
	"github.com/jaywehosl/qd/internal/qsrv"
	"github.com/jaywehosl/qd/internal/store"
	"github.com/jaywehosl/qd/internal/update"
)

type gate struct {
	mu      sync.RWMutex
	allowed map[uint32]bool
	routed  map[uint32]bool
	dev     map[uint32]bool
	core    map[uint32]bool
	told    map[uint32]string
	target  string
	listed  []string
	network string
}

func newGate() *gate {
	return &gate{allowed: map[uint32]bool{}, routed: map[uint32]bool{}, dev: map[uint32]bool{}, core: map[uint32]bool{}, told: map[uint32]string{}}
}

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
	delete(g.routed, id)
	delete(g.dev, id)
	delete(g.core, id)
	delete(g.told, id)
	g.mu.Unlock()
}

func (g *gate) exit(id uint32, allow bool) {
	g.mu.Lock()
	if _, held := g.allowed[id]; held {
		g.allowed[id] = allow
	}
	g.mu.Unlock()
}

func (g *gate) route(id uint32, allow bool) {
	g.mu.Lock()
	if _, held := g.allowed[id]; held {
		g.routed[id] = allow
	}
	g.mu.Unlock()
}

func (g *gate) exits(id uint32) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.allowed[id]
}

func (g *gate) routes(id uint32) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.routed[id]
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
	return qsrv.Grant{Client: raw, AllowExit: allowExit, Steer: g.routed[id], Session: id}, true
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

func loadTLS(certFile, keyFile, authority string, short bool) (*tls.Config, error) {
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
	if short && len(pair.Certificate) > 2 {
		pair.Certificate = pair.Certificate[:2]
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

func (g *gate) builds(id uint32, dev, core bool) {
	g.mu.Lock()
	if _, held := g.allowed[id]; held {
		g.dev[id], g.core[id] = dev, core
	}
	g.mu.Unlock()
}

func (g *gate) policy(target string, releases []string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.target == target && slices.Equal(g.listed, releases) {
		return false
	}
	g.target, g.listed = target, releases
	clear(g.told)
	return true
}

func (g *gate) admit(grant qsrv.Grant, version, build string) bool {
	g.mu.RLock()
	target := g.target
	verdict := update.Judge(version, build, target, g.listed, g.dev[grant.Session], g.core[grant.Session])
	g.mu.RUnlock()
	if verdict != update.Required {
		return true
	}

	seen := version + " " + build
	g.mu.Lock()
	fresh := g.told[grant.Session] != seen
	g.told[grant.Session] = seen
	g.mu.Unlock()
	if fresh {
		fmt.Printf("update     session %d refused a tunnel: client %q %s, the network holds clients to %s\n",
			grant.Session, version, build, target)
	}
	return false
}
