package qsrv

import (
	"context"
	"fmt"
	"hash/fnv"
	"net/http"
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
		if seat := seatOf(r); seat != 0 {
			grant.Session = seat
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

func seatOf(r *http.Request) uint32 {
	text := r.Header.Get(HeaderSeat)
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

	now := time.Now().Unix()
	s := &live{
		grant:   Grant{Client: uuid, AllowExit: grant.AllowExit, Session: grant.Session, Seat: id},
		peer:    uuid,
		since:   now,
		transit: true,
	}
	s.lastSeen.Store(now)
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
