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
	who   identity
	grant Grant
	known bool
	route string
}

type identity struct {
	token   string
	device  string
	node    string
	seat    uint32
	session uint32
}

func identityOf(r *http.Request) identity {
	return identity{
		token:   r.Header.Get(HeaderToken),
		device:  r.Header.Get(HeaderDevice),
		node:    r.Header.Get(HeaderNode),
		seat:    numberIn(r, HeaderSeat),
		session: numberIn(r, HeaderSession),
	}
}

func newSessionContext(ctx context.Context, c *quic.Conn) context.Context {
	return context.WithValue(ctx, sessionKey{}, &held{conn: c})
}

func sessionOf(ctx context.Context) *held {
	s, _ := ctx.Value(sessionKey{}).(*held)
	return s
}

func (h *held) remember(who identity, g Grant) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.who, h.grant, h.known = who, g, true
	h.mu.Unlock()
}

func (h *held) recall() (identity, bool) {
	if h == nil {
		return identity{}, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.who, h.known
}

func (h *held) lastGrant() (Grant, bool) {
	if h == nil {
		return Grant{}, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.grant, h.known
}

func (n *Node) verified(r *http.Request) (Grant, bool) {
	who := identityOf(r)
	h := sessionOf(r.Context())
	fresh := who.token != ""
	if !fresh {
		var ok bool
		if who, ok = h.recall(); !ok {
			return Grant{}, false
		}
	}
	grant, ok := n.grantOf(who)
	if ok && fresh {
		h.remember(who, grant)
	}
	return grant, ok
}

func (n *Node) grantOf(who identity) (Grant, bool) {
	if n.cfg.Verify == nil {
		return Grant{}, false
	}
	grant, ok := n.cfg.Verify(who.token)
	if !ok {
		return Grant{}, false
	}
	grant.Via, grant.Seat = grant.Session, grant.Session
	switch {
	case grant.Peer && who.seat != 0:
		grant.Session, grant.Seat, grant.Client = who.seat, who.seat, who.node
		if who.session != 0 {
			grant.Session = who.session
		}
	case !grant.Peer && who.device != "":
		grant.Seat = seatFor(grant.Session, who.device)
	}
	return grant, true
}

func SeatFor(session uint32, device string) uint32 { return seatFor(session, device) }

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

func (n *Node) peerSession(grant Grant, qc *quic.Conn) *live {
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

	s := n.newLive(Grant{Client: uuid, AllowExit: grant.AllowExit,
		Session: grant.Session, Seat: id, Via: grant.Via}, netip.Prefix{}, uuid, "", true)
	s.conn = qc
	n.held[id] = s
	return s
}

func (n *Node) untilGone(c *quic.Conn, h *held) {
	if c == nil || h == nil {
		return
	}
	<-c.Context().Done()

	grant, ok := h.lastGrant()
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
	if qc := sessionOf(r.Context()).quic(); qc != nil {
		return n.peerSession(grant, qc)
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

	s := n.newLive(grant, n.pool.stream(grant.Seat), r.RemoteAddr, route, false)
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
