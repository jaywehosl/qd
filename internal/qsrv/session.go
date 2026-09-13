package qsrv

import (
	"context"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	quic "github.com/quic-go/quic-go"
)

type sessionKey struct{}

type held struct {
	conn  *quic.Conn
	mu    sync.Mutex
	grant Grant
	known bool
	route string
}

func newSessionContext(ctx context.Context, c *quic.Conn) context.Context {
	return context.WithValue(ctx, sessionKey{}, &held{conn: c})
}

func sessionOf(ctx context.Context) *held {
	s, _ := ctx.Value(sessionKey{}).(*held)
	return s
}

func (h *held) remember(g Grant) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.grant, h.known = g, true
	h.mu.Unlock()
}

func (h *held) recall() (Grant, bool) {
	if h == nil {
		return Grant{}, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.grant, h.known
}

func (n *Node) verified(r *http.Request) (Grant, bool) {
	if token := r.Header.Get(HeaderToken); token != "" && n.cfg.Verify != nil {
		grant, ok := n.cfg.Verify(token)
		if !ok {
			return Grant{}, false
		}
		grant.Seat = grant.Session
		if seat := numberIn(r, HeaderSeat); seat != 0 {
			grant.Session = seat
			if client := numberIn(r, HeaderSession); client != 0 {
				grant.Session = client
			}
			grant.Seat = seat
			grant.Client = r.Header.Get(HeaderNode)
		} else if device := r.Header.Get(HeaderDevice); device != "" {
			grant.Seat = seatFor(grant.Session, device)
		}
		sessionOf(r.Context()).remember(grant)
		return grant, true
	}
	return sessionOf(r.Context()).recall()
}

func seatFor(session uint32, device string) uint32 {
	h := fnv.New32a()
	fmt.Fprintf(h, "%d/%s", session, device)
	seat := h.Sum32()
	if seat == 0 || seat == session {
		seat = session ^ 0x5bf03635
	}
	return seat
}

func numberIn(r *http.Request, header string) uint32 {
	text := r.Header.Get(header)
	if text == "" {
		return 0
	}
	seat, err := strconv.ParseUint(text, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(seat)
}

func (h *held) steer(route string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.route = route
	h.mu.Unlock()
}

func (h *held) heading() string {
	if h == nil {
		return ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.route
}

func (n *Node) peerSession(grant Grant) *live {
	uuid := grant.Client
	if uuid == "" || grant.Seat == 0 {
		return nil
	}
	id := grant.Seat

	n.mu.Lock()
	defer n.mu.Unlock()

	if held := n.held[id]; held != nil {
		return held
	}

	s := newLive(Grant{Client: uuid, AllowExit: grant.AllowExit,
		Session: grant.Session, Seat: id}, netip.Prefix{}, uuid, "")
	s.transit = true
	n.held[id] = s
	return s
}

func (n *Node) untilGone(c *quic.Conn, h *held) {
	if c == nil || h == nil {
		return
	}
	<-c.Context().Done()

	grant, ok := h.recall()
	if !ok || grant.Seat == 0 {
		return
	}

	n.mu.Lock()
	s := n.held[grant.Seat]
	dropped := s != nil && s.transit
	if dropped {
		delete(n.held, grant.Seat)
	}
	n.mu.Unlock()

	if dropped {
		n.cfg.Log("quic      %s left, transit seat %d dropped", grant.Client, grant.Seat)
	}
}

func (h *held) quic() *quic.Conn {
	if h == nil {
		return nil
	}
	return h.conn
}

func (n *Node) counting(grant Grant, r *http.Request, route string) *live {
	n.mu.Lock()
	s := n.held[grant.Seat]
	n.mu.Unlock()
	if s != nil {
		return s
	}
	if sessionOf(r.Context()).quic() != nil {
		return n.peerSession(grant)
	}
	return n.streamSession(grant, r, route)
}

func (n *Node) streamSession(grant Grant, r *http.Request, route string) *live {
	if grant.Seat == 0 {
		return nil
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if s := n.held[grant.Seat]; s != nil {
		return s
	}

	s := newLive(grant, n.pool.stream(grant.Seat), r.RemoteAddr, route)
	s.stream = true
	n.held[grant.Seat] = s
	return s
}

func (n *Node) sweepStreams(quiet time.Duration) {
	cut := time.Now().Add(-quiet).Unix()

	n.mu.Lock()
	going := []*live{}
	for seat, s := range n.held {
		if s.stream && s.lastSeen.Load() < cut {
			going = append(going, s)
			delete(n.held, seat)
		}
	}
	n.mu.Unlock()

	for _, s := range going {
		s.shutFlows()
		n.links.forget(s.grant.Seat)
	}
}

const streamQuiet = 2 * time.Minute

func newLive(grant Grant, address netip.Prefix, peer, route string) *live {
	now := time.Now().Unix()
	s := &live{grant: grant, address: address, peer: peer, since: now}
	s.lastSeen.Store(now)
	s.route.Store(&route)
	return s
}
