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
	"syscall"
	"time"

	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"golang.org/x/net/http2"

	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink/quicconn"
	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink/relay"
	"github.com/jaywehosl/quic-diver/internal/roads"
)

type RelayLink struct {
	Weblink   string
	Authority string
}

type Dialer struct {
	keep    func(fd uintptr)
	mu      sync.Mutex
	token   string
	relays  []RelayLink
	held    map[string]*controlLink
	dialing map[string]chan struct{}
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

// NewKept — диалер, чьи сокеты помечаются как исключённые из туннеля. Нужен на
// Android: там управляющий разговор с узлом иначе уходит в туннель сам к себе.
func NewKept(keep func(fd uintptr)) *Dialer {
	return &Dialer{
		keep:    keep,
		held:    map[string]*controlLink{},
		dialing: map[string]chan struct{}{},
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

func (d *Dialer) SetRelays(relays []RelayLink) {
	d.mu.Lock()
	d.relays = append([]RelayLink{}, relays...)
	d.mu.Unlock()
}

func (d *Dialer) relaySnapshot() []RelayLink {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]RelayLink{}, d.relays...)
}

// controlConfig — живучесть управляющего соединения. Данных оно не несёт, зато
// должно быстро замечать перезапуск узла: пингуем часто и сдаёмся рано, иначе
// панель висит на мёртвом соединении.
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
			if l.alive() {
				held, token := l.asker(), d.token
				d.mu.Unlock()
				return held, token, nil
			}
			l.close()
			delete(d.held, endpoint)
		}
		if wait := d.dialing[endpoint]; wait != nil {
			d.mu.Unlock()
			<-wait
			continue
		}

		done := make(chan struct{})
		if d.dialing == nil {
			d.dialing = map[string]chan struct{}{}
		}
		d.dialing[endpoint] = done
		token := d.token
		d.mu.Unlock()

		link, err := dialControl(endpoint, d.keep, d.relaySnapshot())

		d.mu.Lock()
		delete(d.dialing, endpoint)
		if err == nil {
			d.held[endpoint] = link
		}
		d.mu.Unlock()
		close(done)

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

func dialControl(endpoint string, keep func(fd uintptr), relays []RelayLink) (*controlLink, error) {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, fmt.Errorf("endpoint %q: %w", endpoint, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), openWait)
	defer cancel()

	round, stop := context.WithCancel(ctx)
	defer stop()

	type finish struct {
		link *controlLink
		err  error
	}
	line := make(chan finish, 3)
	paths := 0

	if !roads.OnlyTCP() {
		paths++
		go func() {
			raw, err := quicconn.Dialer{TLS: &tls.Config{
				ServerName: host,
				NextProtos: []string{http3.NextProtoH3},
			}, QUIC: controlConfig(), Keep: keep}.Dial(round, endpoint)
			if err != nil {
				line <- finish{err: fmt.Errorf("quic: %w", err)}
				return
			}
			conn := raw.(*quicconn.Conn)
			tr := &http3.Transport{EnableDatagrams: true}
			line <- finish{link: &controlLink{tr: tr, conn: conn, cc: tr.NewClientConn(conn.QUIC())}}
		}()
	}

	paths++
	go func() {
		select {
		case <-time.After(roads.HeadStart(endpoint)):
		case <-round.Done():
			line <- finish{err: round.Err()}
			return
		}
		link, err := dialOverTCP(round, endpoint, host, keep)
		if err != nil {
			line <- finish{err: fmt.Errorf("tcp: %w", err)}
			return
		}
		line <- finish{link: link}
	}()

	if relayReachable(relays) {
		paths++
		go func() {
			fora := relayHeadStart
			if roads.RelayMode() {
				fora = 0
			}
			select {
			case <-time.After(fora):
			case <-round.Done():
				line <- finish{err: round.Err()}
				return
			}
			for _, link := range relays {
				if link.Weblink == "" || link.Authority == "" {
					continue
				}
				rl, err := dialControlRelay(link, keep)
				if err != nil {
					line <- finish{err: fmt.Errorf("relay: %w", err)}
					return
				}
				line <- finish{link: rl}
				return
			}
			line <- finish{err: errors.New("no relay answered")}
		}()
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

func relayReachable(relays []RelayLink) bool {
	for _, l := range relays {
		if l.Weblink != "" && l.Authority != "" {
			return true
		}
	}
	return false
}

func dialControlRelay(link RelayLink, keep func(fd uintptr)) (*controlLink, error) {
	host, _, err := net.SplitHostPort(link.Authority)
	if err != nil {
		return nil, err
	}
	sess := relay.New(relay.Config{Public: link.Weblink, Keep: keep})
	pc := relay.NewPacketConn(sess)
	if err := sess.Start(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	qc, err := quicconn.DialPacketConn(ctx, pc, relay.Peer, &tls.Config{
		ServerName: host, NextProtos: []string{http3.NextProtoH3},
	}, controlConfig())
	if err != nil {
		sess.Stop()
		return nil, err
	}
	tr := &http3.Transport{EnableDatagrams: true}
	return &controlLink{tr: tr, conn: qc, cc: tr.NewClientConn(qc.QUIC()), relay: sess}, nil
}

func dialOverTCP(ctx context.Context, endpoint, host string, keep func(fd uintptr)) (*controlLink, error) {
	dialer := &net.Dialer{}
	if keep != nil {
		dialer.Control = func(_, _ string, rc syscall.RawConn) error {
			return rc.Control(keep)
		}
	}

	raw, err := roads.ReachTCP(ctx, dialer, endpoint)
	if err != nil {
		return nil, err
	}
	held := tls.Client(raw, &tls.Config{ServerName: host, NextProtos: []string{"h2"}})
	if err := held.HandshakeContext(ctx); err != nil {
		raw.Close()
		return nil, err
	}
	if state := held.ConnectionState(); state.NegotiatedProtocol != "h2" {
		held.Close()
		return nil, fmt.Errorf("the node offered %q, not h2", state.NegotiatedProtocol)
	}

	cc, err := (&http2.Transport{}).NewClientConn(held)
	if err != nil {
		held.Close()
		return nil, err
	}
	return &controlLink{tcp: held, h2: cc}, nil
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
