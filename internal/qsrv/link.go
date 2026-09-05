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

type links struct {
	say   func(string, ...any)
	mu    sync.Mutex
	token string
	self  string
	held  map[string]*link
}

func seatKey(endpoint string, seat uint32) string {
	return endpoint + "#" + strconv.FormatUint(uint64(seat), 10)
}

func newLinks(token, self string, say func(string, ...any)) *links {
	return &links{token: token, self: self, say: say, held: map[string]*link{}}
}

func (ls *links) to(endpoint string, seat uint32) *link {
	key := seatKey(endpoint, seat)

	ls.mu.Lock()
	defer ls.mu.Unlock()

	if l, ok := ls.held[key]; ok {
		return l
	}
	l := &link{endpoint: endpoint, seat: seat, token: ls.token, self: ls.self}
	ls.held[key] = l
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
	ls.held = map[string]*link{}
}

func (ls *links) drop(endpoint string, seat uint32) {
	key := seatKey(endpoint, seat)

	ls.mu.Lock()
	l, ok := ls.held[key]
	if ok {
		delete(ls.held, key)
	}
	ls.mu.Unlock()

	if ok {
		l.close()
	}
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

func (ls *links) hold(key string) {
	ls.mu.Lock()
	l := ls.held[key]
	ls.mu.Unlock()

	if l != nil {
		l.flows.Add(1)
	}
}

func (ls *links) release(key string) {
	ls.mu.Lock()
	l := ls.held[key]
	ls.mu.Unlock()

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
	for where, l := range ls.held {
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
		delete(ls.held, where)
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
		}
	}
}

const (
	linkSweep = 5 * time.Second
	linkQuiet = 15 * time.Second
)
