package qwire

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"golang.org/x/net/http2"

	"github.com/jaywehosl/quic-diver/internal/qsrv/transport/cip"
	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink/quicconn"
	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink/relay"
	"github.com/jaywehosl/quic-diver/internal/roads"
)

type Dialer struct {
	keep    func(fd uintptr)
	mu      sync.Mutex
	token   string
	relays  []relay.Link
	held    map[string]*controlLink
	dialing map[string]*dialing
}

type dialing struct {
	done chan struct{}
	err  error
}

type controlLink struct {
	tr   *http3.Transport
	conn *quicconn.Conn
	cc   *http3.ClientConn

	tcp     net.Conn
	h2      *http2.ClientConn
	relay   *relay.Session
	weblink string
}

func (l *controlLink) follows(endpoint string) bool {
	p, pinned := roads.Following(endpoint)
	switch {
	case !pinned:
		return true
	case p.Relay != "":
		return l.weblink != "" && relay.Doc(l.weblink) == relay.Doc(p.Relay)
	case p.OverTCP:
		return l.h2 != nil
	default:
		return l.cc != nil && l.relay == nil
	}
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
		MaxIdleTimeout:  10 * time.Second,
		KeepAlivePeriod: 3 * time.Second,
	}
}

const openWait = 5 * time.Second

func (d *Dialer) conn(endpoint string) (asker, string, error) {
	for {
		d.mu.Lock()
		if l, ok := d.held[endpoint]; ok {
			if l.alive() && l.follows(endpoint) {
				held, token := l.asker(), d.token
				d.mu.Unlock()
				return held, token, nil
			}
			l.close()
			delete(d.held, endpoint)
		}
		if pending := d.dialing[endpoint]; pending != nil {
			d.mu.Unlock()
			<-pending.done
			if pending.err != nil {
				return nil, "", pending.err
			}
			continue
		}

		pending := &dialing{done: make(chan struct{})}
		d.dialing[endpoint] = pending
		token := d.token
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
			return nil, "", err
		}
		return link.asker(), token, nil
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

	round, stop := context.WithCancel(context.Background())
	defer stop()

	direct, directStop := context.WithTimeout(round, openWait)
	defer directStop()

	pin, pinned := roads.Following(endpoint)
	mine := relaysFor(endpoint, relays)
	if pinned {
		mine = nil
		if pin.Relay != "" {
			mine = []relay.Link{{Authority: endpoint, Weblink: pin.Relay}}
		}
	}

	type finish struct {
		link *controlLink
		err  error
	}
	line := make(chan finish, 2+len(mine))
	paths := 0

	if !roads.OnlyTCP() && (!pinned || (pin.Relay == "" && !pin.OverTCP)) {
		paths++
		go func() {
			conn, err := quicconn.Dialer{TLS: &tls.Config{ServerName: host}, QUIC: controlConfig(), Keep: keep}.Dial(direct, endpoint)
			if err != nil {
				line <- finish{err: fmt.Errorf("quic: %w", err)}
				return
			}
			tr := &http3.Transport{EnableDatagrams: true}
			line <- finish{link: &controlLink{tr: tr, conn: conn, cc: tr.NewClientConn(conn.QUIC())}}
		}()
	}

	if !pinned || (pin.Relay == "" && pin.OverTCP) {
		paths++
		fora := roads.HeadStart(endpoint)
		if pinned {
			fora = 0
		}
		go func() {
			select {
			case <-time.After(fora):
			case <-direct.Done():
				line <- finish{err: direct.Err()}
				return
			}
			conn, cc, err := cip.ReachH2(direct, endpoint, keep)
			if err != nil {
				line <- finish{err: fmt.Errorf("tcp: %w", err)}
				return
			}
			line <- finish{link: &controlLink{tcp: conn, h2: cc}}
		}()
	}

	fora := relayHeadStart
	if pinned || roads.RelayMode() {
		fora = 0
	}
	for _, link := range mine {
		paths++
		go func(link relay.Link) {
			select {
			case <-time.After(fora):
			case <-round.Done():
				line <- finish{err: round.Err()}
				return
			}
			rl, err := dialControlRelay(round, link, keep)
			if err != nil {
				line <- finish{err: fmt.Errorf("relay %s: %w", link.Weblink, err)}
				return
			}
			line <- finish{link: rl}
		}(link)
	}

	if paths == 0 {
		return nil, fmt.Errorf("no path to %s", pin)
	}

	var refused []string
	for i := 0; i < paths; i++ {
		got := <-line
		if got.err != nil {
			refused = append(refused, got.err.Error())
			continue
		}
		roads.Remember(endpoint, got.link.cc == nil)

		left := paths - i - 1
		go func() {
			for k := 0; k < left; k++ {
				if late := <-line; late.link != nil {
					late.link.close()
				}
			}
		}()
		return got.link, nil
	}
	return nil, errors.New(strings.Join(refused, " / "))
}

const relayHeadStart = 800 * time.Millisecond

func relaysFor(endpoint string, relays []relay.Link) []relay.Link {
	var out []relay.Link
	for _, l := range relays {
		if l.Weblink != "" && l.Authority == endpoint {
			out = append(out, l)
		}
	}
	return out
}

const relayWait = 20 * time.Second

func dialControlRelay(ctx context.Context, link relay.Link, keep func(fd uintptr)) (*controlLink, error) {
	sess := relay.New(relay.Config{Public: link.Weblink, Keep: keep})
	round, cancel := context.WithTimeout(ctx, relayWait)
	defer cancel()
	qc, err := quicconn.OverRelay(round, sess, link.Authority, controlConfig())
	if err != nil {
		return nil, err
	}
	tr := &http3.Transport{EnableDatagrams: true}
	return &controlLink{tr: tr, conn: qc, cc: tr.NewClientConn(qc.QUIC()), relay: sess, weblink: link.Weblink}, nil
}

func (d *Dialer) drop(endpoint string) {
	d.mu.Lock()
	defer d.mu.Unlock()
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
