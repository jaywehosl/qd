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
	"github.com/jaywehosl/quic-diver/internal/roads"
)

type Dialer struct {
	keep    func(fd uintptr)
	mu      sync.Mutex
	token   string
	held    map[string]*controlLink
	dialing map[string]chan struct{}
}

type controlLink struct {
	tr   *http3.Transport
	conn *quicconn.Conn
	cc   *http3.ClientConn

	tcp net.Conn
	h2  *http2.ClientConn
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

		link, err := dialControl(endpoint, d.keep)

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

func dialControl(endpoint string, keep func(fd uintptr)) (*controlLink, error) {
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
	line := make(chan finish, 2)
	paths := 2
	if roads.OnlyTCP() {
		paths = 1
	}

	if !roads.OnlyTCP() {
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

	var refused []string
	for i := 0; i < paths; i++ {
		got := <-line
		if got.err != nil {
			refused = append(refused, got.err.Error())
			continue
		}
		roads.Remember(endpoint, got.link.cc == nil)

		// Второй путь, если он всё же добежал, закрываем: иначе к узлу остаётся
		// висеть лишнее соединение, по которому никто не спросит.
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
}
