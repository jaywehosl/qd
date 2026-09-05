package qwire

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink/quicconn"
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
}

func New() *Dialer { return NewKept(nil) }

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

type stopper context.CancelFunc

func (s stopper) Close() error {
	s()
	return nil
}

func controlConfig() *quic.Config {
	return &quic.Config{
		EnableDatagrams: true,
		MaxIdleTimeout:  10 * time.Second,
		KeepAlivePeriod: 3 * time.Second,
	}
}

const openWait = 5 * time.Second

func (d *Dialer) conn(endpoint string) (*http3.ClientConn, string, error) {
	for {
		d.mu.Lock()
		if l, ok := d.held[endpoint]; ok {
			select {
			case <-l.conn.QUIC().Context().Done():
				l.close()
				delete(d.held, endpoint)
			default:
				cc, token := l.cc, d.token
				d.mu.Unlock()
				return cc, token, nil
			}
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
		return link.cc, token, nil
	}
}

func dialControl(endpoint string, keep func(fd uintptr)) (*controlLink, error) {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, fmt.Errorf("endpoint %q: %w", endpoint, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), openWait)
	defer cancel()

	raw, err := quicconn.Dialer{TLS: &tls.Config{
		ServerName: host,
		NextProtos: []string{http3.NextProtoH3},
	}, QUIC: controlConfig(), Keep: keep}.Dial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	conn := raw.(*quicconn.Conn)

	tr := &http3.Transport{EnableDatagrams: true}
	cc := tr.NewClientConn(conn.QUIC())
	return &controlLink{tr: tr, conn: conn, cc: cc}, nil
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
	if l.tr != nil {
		l.tr.Close()
	}
	if l.conn != nil {
		l.conn.Close()
	}
}

type bothClosers []io.Closer

func (c bothClosers) Close() error {
	var last error
	for _, one := range c {
		if err := one.Close(); err != nil {
			last = err
		}
	}
	return last
}
