package qsrv

import (
	"context"
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
		if seat := seatOf(r); seat != 0 {
			grant.Session = seat
			grant.Client = r.Header.Get(HeaderNode)
		}
		sessionOf(r.Context()).remember(grant)
		return grant, true
	}
	return sessionOf(r.Context()).recall()
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
	if uuid == "" || grant.Session == 0 {
		return nil
	}
	id := grant.Session

	n.mu.Lock()
	defer n.mu.Unlock()

	if held := n.held[id]; held != nil {
		return held
	}

	now := time.Now().Unix()
	s := &live{
		grant:   Grant{Client: uuid, AllowExit: grant.AllowExit, Session: id},
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
	if !ok || grant.Session == 0 {
		n.cfg.Log("quic      a connection closed with nothing to forget (known %v)", ok)
		return
	}

	n.mu.Lock()
	s := n.held[grant.Session]
	dropped := s != nil && s.transit
	if dropped {
		delete(n.held, grant.Session)
	}
	n.mu.Unlock()

	n.cfg.Log("quic      %s left, transit %d dropped %v (held %v)", grant.Client, grant.Session, dropped, s != nil)
}

func (h *held) quic() *quic.Conn {
	if h == nil {
		return nil
	}
	return h.conn
}
