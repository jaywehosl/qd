package qwire

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"golang.org/x/net/http2"

	"github.com/jaywehosl/qd/internal/pace"
	"github.com/jaywehosl/qd/internal/qsrv"
	"github.com/jaywehosl/qd/internal/qsrv/transport/cip"
	"github.com/jaywehosl/qd/internal/qsrv/uplink/quicconn"
	"github.com/jaywehosl/qd/internal/qsrv/uplink/relay"
	"github.com/jaywehosl/qd/internal/roads"
	"github.com/jaywehosl/qd/internal/roots"
)

type Dialer struct {
	keep    func(fd uintptr)
	mu      sync.Mutex
	token   string
	relays  []relay.Link
	held    map[string]*controlLink
	dialing map[string]*dialing
	misses  map[string]int
	riding  map[string]*ride
}

type dialing struct {
	done chan struct{}
	err  error
}

type controlLink struct {
	tr   *http3.Transport
	conn *quicconn.Conn
	cc   *http3.ClientConn

	tcp   net.Conn
	h2    *http2.ClientConn
	relay *relay.Session
}

type asker interface {
	RoundTrip(*http.Request) (*http.Response, error)
}

func (l *controlLink) asker() asker {
	if l.cc != nil {
		return l.cc
	}
	return l.h2
}

func New() *Dialer { return NewKept(nil) }

func NewKept(keep func(fd uintptr)) *Dialer {
	return &Dialer{
		keep:    keep,
		held:    map[string]*controlLink{},
		dialing: map[string]*dialing{},
		misses:  map[string]int{},
		riding:  map[string]*ride{},
	}
}

func (d *Dialer) SetToken(token string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.token == token {
		return
	}
	d.token = token
	for where, l := range d.held {
		l.close()
		delete(d.held, where)
	}
}

func (d *Dialer) SetRelays(relays []relay.Link) {
	d.mu.Lock()
	d.relays = append([]relay.Link{}, relays...)
	d.mu.Unlock()
}

func (d *Dialer) relaySnapshot() []relay.Link {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]relay.Link{}, d.relays...)
}

func controlConfig() *quic.Config {
	return &quic.Config{
		EnableDatagrams: true,
		MaxIdleTimeout:  45 * time.Second,
		KeepAlivePeriod: 15 * time.Second,
	}
}

func (d *Dialer) conn(endpoint string) (asker, func(http.Header), error) {
	for {
		d.mu.Lock()
		if held := d.riding[endpoint]; held != nil && held.alive() {
			d.mu.Unlock()
			return held.via, held.sign, nil
		}
		token := d.token
		sign := func(h http.Header) { h.Set(qsrv.HeaderToken, token) }
		if l, ok := d.held[endpoint]; ok {
			if l.alive() {
				held := l.asker()
				d.mu.Unlock()
				return held, sign, nil
			}
			l.close()
			delete(d.held, endpoint)
		}

		if pending := d.dialing[endpoint]; pending != nil {
			d.mu.Unlock()
			<-pending.done
			if pending.err != nil {
				return nil, nil, pending.err
			}
			continue
		}

		pending := &dialing{done: make(chan struct{})}
		d.dialing[endpoint] = pending
		d.mu.Unlock()

		link, err := dialControl(endpoint, d.keep, d.relaySnapshot())

		d.mu.Lock()
		delete(d.dialing, endpoint)
		if err == nil {
			d.held[endpoint] = link
		}
		pending.err = err
		d.mu.Unlock()
		close(pending.done)

		if err != nil {
			return nil, nil, err
		}
		return link.asker(), sign, nil
	}
}

func (l *controlLink) alive() bool {
	if l.cc != nil {
		select {
		case <-l.conn.QUIC().Context().Done():
			return false
		default:
			return true
		}
	}
	return l.h2 != nil && l.h2.CanTakeNewRequest()
}

func dialControl(endpoint string, keep func(fd uintptr), relays []relay.Link) (*controlLink, error) {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, fmt.Errorf("endpoint %q: %w", endpoint, err)
	}

	ctx, stop := context.WithTimeout(context.Background(), pace.ControlWait)
	defer stop()

	var tries []roads.Try[*controlLink]
	if !roads.OnlyTCP() {
		tries = append(tries, roads.Try[*controlLink]{Rung: roads.QUIC, Run: func(ctx context.Context) (*controlLink, error) {
			conn, err := quicconn.Dialer{TLS: &tls.Config{ServerName: host, RootCAs: roots.Pool()}, QUIC: controlConfig(), Keep: keep}.Dial(ctx, endpoint)
			if err != nil {
				return nil, fmt.Errorf("quic: %w", err)
			}
			tr := &http3.Transport{EnableDatagrams: true}
			return &controlLink{tr: tr, conn: conn, cc: tr.NewClientConn(conn.QUIC())}, nil
		}})
	}
	tries = append(tries, roads.Try[*controlLink]{Rung: roads.TCP, Run: func(ctx context.Context) (*controlLink, error) {
		conn, cc, err := cip.ReachH2(ctx, endpoint, keep)
		if err != nil {
			return nil, fmt.Errorf("tcp: %w", err)
		}
		return &controlLink{tcp: conn, h2: cc}, nil
	}})
	for _, link := range relaysFor(endpoint, relays) {
		tries = append(tries, roads.Try[*controlLink]{Rung: roads.Relay, Run: func(ctx context.Context) (*controlLink, error) {
			sess := relay.New(relay.Config{Public: link.Weblink, Keep: keep})
			qc, err := quicconn.OverRelay(ctx, sess, link.Authority, controlConfig(), nil)
			if err != nil {
				return nil, fmt.Errorf("relay %s: %w", link.Weblink, err)
			}
			tr := &http3.Transport{EnableDatagrams: true}
			return &controlLink{tr: tr, conn: qc, cc: tr.NewClientConn(qc.QUIC()), relay: sess}, nil
		}})
	}

	link, _, err := roads.Climb(ctx, endpoint, tries, func(late *controlLink) { late.close() }, nil)
	return link, err
}

func relaysFor(endpoint string, relays []relay.Link) []relay.Link {
	var out []relay.Link
	for _, l := range relays {
		if l.Weblink != "" && l.Authority == endpoint {
			out = append(out, l)
		}
	}
	return out
}

type ride struct {
	via   asker
	alive func() bool
	sign  func(http.Header)
}

func (d *Dialer) Ride(endpoint string, via asker, alive func() bool, sign func(http.Header)) (release func()) {
	held := &ride{via: via, alive: alive, sign: sign}
	d.mu.Lock()
	d.riding[endpoint] = held
	if l, ok := d.held[endpoint]; ok {
		l.close()
		delete(d.held, endpoint)
	}
	d.mu.Unlock()
	return func() {
		d.mu.Lock()
		if d.riding[endpoint] == held {
			delete(d.riding, endpoint)
		}
		d.mu.Unlock()
	}
}

func (d *Dialer) drop(endpoint string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.misses, endpoint)
	if l, ok := d.held[endpoint]; ok {
		l.close()
		delete(d.held, endpoint)
	}
}

func (l *controlLink) close() {
	if l.tcp != nil {
		l.tcp.Close()
	}
	if l.tr != nil {
		l.tr.Close()
	}
	if l.conn != nil {
		l.conn.Close()
	}
	if l.relay != nil {
		l.relay.Stop()
	}
}

const missesToDrop = 3

func (d *Dialer) missed(endpoint string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.misses[endpoint]++
	return d.misses[endpoint] >= missesToDrop
}

func (d *Dialer) answered(endpoint string) {
	d.mu.Lock()
	if d.misses[endpoint] != 0 {
		delete(d.misses, endpoint)
	}
	d.mu.Unlock()
}

func (d *Dialer) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for endpoint, l := range d.held {
		l.close()
		delete(d.held, endpoint)
	}
	clear(d.misses)
}
