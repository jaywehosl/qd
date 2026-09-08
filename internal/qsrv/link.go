package qsrv

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go/http3"

	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink/quicconn"
)

const peerDialTimeout = 8 * time.Second

type link struct {
	flows atomic.Int64
	quiet atomic.Int64
	// won — эта связь выиграла гонку для своего места. Живёт здесь, а не в
	// карте рядом: умирает вместе со связью и разойтись с ней не может.
	won atomic.Bool

	mu       sync.Mutex
	endpoint string
	seat     uint32
	self     string
	token    string
	cc       *http3.ClientConn
	tr       *http3.Transport
	conn     *quicconn.Conn
	dialing  chan struct{}
}

func (l *link) connect(ctx context.Context) (*http3.ClientConn, error) {
	for {
		l.mu.Lock()
		if l.cc != nil {
			select {
			case <-l.conn.QUIC().Context().Done():
				l.dropLocked()
			default:
				cc := l.cc
				l.mu.Unlock()
				return cc, nil
			}
		}
		if wait := l.dialing; wait != nil {
			l.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		done := make(chan struct{})
		l.dialing = done
		l.mu.Unlock()

		cc, tr, conn, err := l.dial(ctx)

		l.mu.Lock()
		l.dialing = nil
		if err == nil {
			l.cc, l.tr, l.conn = cc, tr, conn
		}
		l.mu.Unlock()
		close(done)

		if err != nil {
			return nil, err
		}
		return cc, nil
	}
}

func (l *link) dial(ctx context.Context) (*http3.ClientConn, *http3.Transport, *quicconn.Conn, error) {
	host, _, err := net.SplitHostPort(l.endpoint)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("peer %q: %w", l.endpoint, err)
	}

	dialCtx, cancel := context.WithTimeout(ctx, peerDialTimeout)
	defer cancel()

	tlsConf := &tls.Config{ServerName: host, NextProtos: []string{http3.NextProtoH3}}
	raw, err := quicconn.Dialer{TLS: tlsConf}.Dial(dialCtx, l.endpoint)
	if err != nil {
		return nil, nil, nil, err
	}
	conn := raw.(*quicconn.Conn)

	tr := &http3.Transport{EnableDatagrams: true}
	cc := tr.NewClientConn(conn.QUIC())

	if err := greetPeer(dialCtx, cc, l.token, l.self, l.seat, "https://"+l.endpoint+AuthPath); err != nil {
		tr.Close()
		conn.Close()
		return nil, nil, nil, err
	}
	return cc, tr, conn, nil
}

func (l *link) dropLocked() {
	if l.tr != nil {
		l.tr.Close()
	}
	if l.conn != nil {
		l.conn.Close()
	}
	l.cc, l.tr, l.conn = nil, nil, nil
}

func (l *link) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dropLocked()
}

func greetPeer(ctx context.Context, cc *http3.ClientConn, token, self string, seat uint32, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set(HeaderToken, peerToken(token, self))
	if self != "" {
		req.Header.Set(HeaderNode, self)
	}
	if seat != 0 {
		req.Header.Set(HeaderSeat, strconv.FormatUint(uint64(seat), 10))
	}

	rsp, err := cc.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("peer handshake: %w", err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("peer refused the network key")
	}
	return nil
}

// where — куда и для кого связь. Раньше ключом была склеенная строка, и она
// строилась заново на каждом флоу; здесь склеивать нечего.
type where struct {
	endpoint string
	seat     uint32
}

type links struct {
	say   func(string, ...any)
	mu    sync.Mutex
	token string
	self  string
	held  map[where]*link
}

func newLinks(token, self string, say func(string, ...any)) *links {
	return &links{token: token, self: self, say: say, held: map[where]*link{}}
}

// standing отдаёт уже живую связь с одним из кандидатов: сперва ту, что выиграла
// прошлую гонку для этого места, иначе первую живую по порядку узлов. Порядок
// стабилен, поэтому выход не скачет от флоу к флоу.
//
// Победа — свойство самой связи, а не отдельная карта рядом. Пока она жила
// отдельно, её приходилось чистить руками вместе со связью, и стоило забыть —
// память показывала один выход, а трафик шёл в другой.
func (ls *links) standing(runners []Peer, seat uint32) (*http3.ClientConn, string, bool) {
	for _, p := range runners {
		l := ls.find(where{p.Endpoint, seat})
		if l == nil || !l.won.Load() {
			continue
		}
		if cc, ok := l.alive(); ok {
			return cc, p.Endpoint, true
		}
	}

	for _, p := range runners {
		l := ls.find(where{p.Endpoint, seat})
		if l == nil {
			continue
		}
		if cc, ok := l.alive(); ok {
			ls.chose(seat, p.Endpoint)
			return cc, p.Endpoint, true
		}
	}
	return nil, "", false
}

func (ls *links) find(at where) *link {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.held[at]
}

func (l *link) alive() (*http3.ClientConn, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cc == nil || l.conn == nil {
		return nil, false
	}
	select {
	case <-l.conn.QUIC().Context().Done():
		return nil, false
	default:
		return l.cc, true
	}
}

// chose помечает победителя и снимает пометку с остальных связей этого места:
// победитель у места ровно один.
func (ls *links) chose(seat uint32, endpoint string) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for at, l := range ls.held {
		if at.seat == seat {
			l.won.Store(at.endpoint == endpoint)
		}
	}
}

func (ls *links) to(at where) *link {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if l, ok := ls.held[at]; ok {
		return l
	}
	l := &link{endpoint: at.endpoint, seat: at.seat, token: ls.token, self: ls.self}
	ls.held[at] = l
	return l
}

func (ls *links) setToken(token string) {
	ls.mu.Lock()
	ls.token = token
	for _, l := range ls.held {
		l.mu.Lock()
		l.token = token
		l.mu.Unlock()
	}
	ls.mu.Unlock()
}

func (ls *links) closeAll() {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for _, l := range ls.held {
		l.close()
	}
	ls.held = map[where]*link{}
}

func (ls *links) forget(seat uint32) {
	ls.mu.Lock()
	going := []*link{}
	for key, l := range ls.held {
		if l.seat == seat {
			going = append(going, l)
			delete(ls.held, key)
		}
	}
	ls.mu.Unlock()

	for _, l := range going {
		l.close()
	}
}

func peerToken(token, self string) string {
	if self != "" {
		return self
	}
	return token
}

func (ls *links) hold(at where) {
	if l := ls.find(at); l != nil {
		l.flows.Add(1)
	}
}

func (ls *links) release(at where) {
	l := ls.find(at)
	if l == nil {
		return
	}
	if l.flows.Add(-1) <= 0 {
		l.quiet.Store(time.Now().Unix())
	}
}

func (ls *links) sweep(quiet time.Duration) {
	now := time.Now().Unix()

	ls.mu.Lock()
	idle := []*link{}
	for at, l := range ls.held {
		if l.flows.Load() > 0 {
			continue
		}
		since := l.quiet.Load()
		if since == 0 {
			l.quiet.Store(now)
			continue
		}
		if now-since < int64(quiet.Seconds()) {
			continue
		}
		idle = append(idle, l)
		delete(ls.held, at)
	}
	ls.mu.Unlock()

	for _, l := range idle {
		l.close()
	}
	if len(idle) > 0 && ls.say != nil {
		ls.say("quic      closed %d idle links to peers", len(idle))
	}
}

func (n *Node) sweepLinks(ctx context.Context) {
	tick := time.NewTicker(linkSweep)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			n.links.sweep(linkQuiet)
			n.sweepStreams(streamQuiet)
		}
	}
}

const (
	linkSweep = 5 * time.Second
	linkQuiet = 15 * time.Second
)
