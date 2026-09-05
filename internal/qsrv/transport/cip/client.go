// Package cip — слой connect-ip (RFC 9484) модели B: транспорт сырых IP-пакетов
// между клиентом и узлом поверх QUIC/HTTP3.
//
// Стек клиента:
//
//	packet.Source ──> cip.Client ──> connectip.Conn (ReadPacket/WritePacket)
//	                                       │
//	                              http3.ClientConn
//	                                       │
//	                          quicconn.Conn.QUIC() (*quic.Conn + миграция arch4)
//
// Миграция живёт на quic-объекте (quicconn.Conn.Migrate), поэтому http3 и
// connect-ip переживают смену пути прозрачно.
package cip

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/netip"

	connectip "github.com/quic-go/connect-ip-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/yosida95/uritemplate/v3"

	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink/quicconn"
)

// Client — клиентский конец connect-ip туннеля.
type Client struct {
	qc *quicconn.Conn
	h3 *http3.Transport
	cc *http3.ClientConn
	ip *connectip.Conn

	auth   string
	device string
	token  string
}

// H3Conn — то же HTTP/3-соединение, поверх которого поднят connect-ip. Гибрид
// открывает через него CONNECT-стримы для TCP-флоу, поэтому и датаграммы, и
// стримы делят одну QUIC-сессию: один handshake, один congestion-control.
func (c *Client) H3Conn() *http3.ClientConn { return c.cc }

// Dial устанавливает туннель к узлу.
//   - endpoint: host:port (host — доменное имя, arch3);
//   - tmpl: URI Template connect-ip эндпоинта на узле;
//   - tlsConf: TLS клиента (ALPN дополняется h3; должен нести ServerName).
func Dial(ctx context.Context, endpoint string, tmpl *uritemplate.Template, tlsConf *tls.Config) (*Client, *http.Response, error) {
	qcAny, err := quicconn.Dialer{TLS: ensureH3(tlsConf)}.Dial(ctx, endpoint)
	if err != nil {
		return nil, nil, err
	}
	qc := qcAny.(*quicconn.Conn)

	h3tr := &http3.Transport{EnableDatagrams: true}
	cc := h3tr.NewClientConn(qc.QUIC())

	ipConn, rsp, err := connectip.Dial(ctx, cc, tmpl)
	if err != nil {
		h3tr.Close()
		qc.Close()
		return nil, nil, err
	}
	return &Client{qc: qc, h3: h3tr, cc: cc, ip: ipConn}, rsp, nil
}

// WritePacket отправляет сырой IP-пакет в туннель. Если пакет превышает путь,
// возвращает готовый ICMP (PTB) для отправки обратно источнику — MTU-инженерия B.
func (c *Client) WritePacket(b []byte) (icmp []byte, err error) { return c.ip.WritePacket(b) }

// ReadPacket читает один сырой IP-пакет из туннеля в b.
func (c *Client) ReadPacket(b []byte) (int, error) { return c.ip.ReadPacket(b) }

// LocalPrefixes — адреса, назначенные узлом этому клиенту (ADDRESS_ASSIGN).
func (c *Client) LocalPrefixes(ctx context.Context) ([]netip.Prefix, error) {
	return c.ip.LocalPrefixes(ctx)
}

// Migrate переносит сессию на новый локальный сокет без разрыва (arch4).
func (c *Client) Migrate(ctx context.Context, laddr *net.UDPAddr) error {
	return c.qc.Migrate(ctx, laddr)
}

// Close закрывает туннель и весь стек под ним.
func (c *Client) Close() error {
	err := c.ip.Close()
	c.h3.Close()
	c.qc.Close()
	return err
}

// ensureH3 гарантирует ALPN "h3" (обязателен для connect-ip поверх HTTP/3).
func ensureH3(t *tls.Config) *tls.Config {
	if t == nil {
		t = &tls.Config{}
	} else {
		t = t.Clone()
	}
	if len(t.NextProtos) == 0 {
		t.NextProtos = []string{http3.NextProtoH3}
	}
	return t
}

func DialAuth(ctx context.Context, endpoint string, tmpl *uritemplate.Template, tlsConf *tls.Config, token, device, route, authURL string, keep func(fd uintptr)) (*Client, *http.Response, error) {
	qcAny, err := quicconn.Dialer{TLS: ensureH3(tlsConf), Keep: keep}.Dial(ctx, endpoint)
	if err != nil {
		return nil, nil, err
	}
	qc := qcAny.(*quicconn.Conn)

	h3tr := &http3.Transport{EnableDatagrams: true}
	cc := h3tr.NewClientConn(qc.QUIC())

	if authURL != "" {
		if err := greet(ctx, cc, token, device, route, authURL); err != nil {
			h3tr.Close()
			qc.Close()
			return nil, nil, err
		}
	}

	ipConn, rsp, err := connectip.Dial(ctx, cc, tmpl)
	if err != nil {
		h3tr.Close()
		qc.Close()
		return nil, nil, err
	}
	return &Client{qc: qc, h3: h3tr, cc: cc, ip: ipConn, auth: authURL, token: token, device: device}, rsp, nil
}

func greet(ctx context.Context, cc *http3.ClientConn, token, device, route, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set(tokenHeader, token)
	if device != "" {
		req.Header.Set(deviceHeader, device)
	}
	if route == "" {
		route = hereExit
	}
	req.Header.Set(routeHeader, route)

	rsp, err := cc.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("the node did not answer: %w", err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("the node refused this subscription")
	}
	return nil
}

const (
	tokenHeader  = "Qd-Token"
	routeHeader  = "Qd-Route"
	deviceHeader = "Qd-Device"
	hereExit     = "here"
)

func (c *Client) Steer(ctx context.Context, route string) error {
	if c.auth == "" {
		return nil
	}
	return greet(ctx, c.cc, c.token, c.device, route, c.auth)
}

func (c *Client) WritePacketMarked(b []byte, mark uint64) (icmp []byte, err error) {
	return c.ip.WritePacketMarked(b, mark)
}

func (c *Client) ReadPacketMarked(b []byte) (int, uint64, error) {
	return c.ip.ReadPacketMarked(b)
}

// Alive — жива ли QUIC-сессия. Сама она о смерти не сообщает: наш keepalive
// шлёт PING и держит idle-таймер живым, даже когда с той стороны давно никого
// нет, поэтому спрашивать надо у соединения, а не ждать таймаута.
func (c *Client) Alive() bool {
	qc := c.qc.QUIC()
	if qc == nil {
		return false
	}
	return qc.Context().Err() == nil
}

// Ask спрашивает узел по тому же соединению: отвечает ли он и помнит ли эту
// подписку. Дешевле миграции — ни нового сокета, ни проверки пути, — и честнее
// счётчиков: молчащий туннель от мёртвого иначе не отличить.
func (c *Client) Ask(ctx context.Context, route string) error {
	if !c.Alive() {
		return fmt.Errorf("the session is closed")
	}
	if c.auth == "" {
		return fmt.Errorf("nothing to ask: this client never greeted the node")
	}
	return greet(ctx, c.cc, c.token, c.device, route, c.auth)
}

func (c *Client) DatagramLimit() int {
	_ = c.qc.SendDatagram(make([]byte, 1<<16))
	return c.qc.MaxDatagramSize()
}
