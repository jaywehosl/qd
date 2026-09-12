// Package quicconn — реализация uplink.Conn/Dialer поверх quic-go (v0.60).
//
// Это транспортный кирпич модели B: одна QUIC-сессия несёт весь трафик клиента.
// Датаграммы (RFC 9221) переносят IP-пакеты (позже — обёрнутые в connect-ip),
// потоки — крупные payload и будущая модель A. Conn держит собственный
// quic.Transport, поэтому умеет мигрировать на новый локальный сокет без разрыва
// сессии (arch4): смена Wi-Fi↔LTE, пересборка PPPoE.
package quicconn

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	quic "github.com/quic-go/quic-go"

	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink"
)

// ALPN — идентификатор протокола QUIC Diver в TLS-хендшейке.
const ALPN = "qd/1"

// defaultMaxDatagram — консервативная оценка лимита датаграммы до первого
// уточнения из DatagramTooLargeError (IPv6 min MTU 1280 минус заголовки).
const defaultMaxDatagram = 1200

// udpBufSize — размер буферов UDP-сокета. Слишком малые теряют датаграммы при
// всплесках; слишком большие дают bufferbloat (очередь копится → RTT под
// нагрузкой растёт). Ориентир — покрыть BDP (800Мбит×15мс ≈ 1.4МБ) с запасом (4МБ).
// На 2МБ Windows отдаёт WSAENOBUFS под BRUTAL, и quic-go роняет по ней сессию.
const udpBufSize = 4 << 20

func setUDPBuffers(pc *net.UDPConn) {
	_ = pc.SetReadBuffer(udpBufSize)
	_ = pc.SetWriteBuffer(udpBufSize)
}

// transportSocket — пара «транспорт + его сокет» для одного сетевого пути.
type transportSocket struct {
	tr *quic.Transport
	pc net.PacketConn
}

// Conn — одна QUIC-сессия до узла.
type Conn struct {
	qc *quic.Conn

	mu       sync.Mutex
	tr       *quic.Transport   // активный транспорт (сокет текущего пути)
	pc       net.PacketConn    // сокет активного пути
	prev     []transportSocket // старые пути после миграции, живут до Close
	remote   net.Addr
	keep     func(fd uintptr)
	maxDgram atomic.Int64
}

// SendDatagram шлёт ненадёжную датаграмму. При превышении лимита обновляет
// известный MaxDatagramSize (для MTU-инженерии модели B) и возвращает ошибку.
func (c *Conn) SendDatagram(b []byte) error {
	err := c.qc.SendDatagram(b)
	var tooLarge *quic.DatagramTooLargeError
	if errors.As(err, &tooLarge) {
		c.maxDgram.Store(tooLarge.MaxDatagramPayloadSize)
	}
	return err
}

// RecvDatagram принимает ненадёжную датаграмму.
func (c *Conn) RecvDatagram(ctx context.Context) ([]byte, error) {
	return c.qc.ReceiveDatagram(ctx)
}

// OpenStream открывает надёжный двунаправленный поток.
func (c *Conn) OpenStream(ctx context.Context) (uplink.Stream, error) {
	s, err := c.qc.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return s, nil // *quic.Stream реализует io.ReadWriteCloser
}

// MaxDatagramSize — текущий известный лимит полезной датаграммы в байтах.
func (c *Conn) MaxDatagramSize() int {
	if v := c.maxDgram.Load(); v > 0 {
		return int(v)
	}
	return defaultMaxDatagram
}

// QUIC возвращает нижележащее *quic.Conn для слоёв поверх (http3/connect-ip).
// Миграция (Migrate) работает на этом же объекте, поэтому слои сверху переживают
// смену пути прозрачно — объект conn при миграции не пересоздаётся.
func (c *Conn) QUIC() *quic.Conn { return c.qc }

// Migrate переносит сессию на новый локальный UDP-сокет без разрыва (arch4).
// Открывает сокет на laddr, добавляет путь, валидирует его (PATH_CHALLENGE),
// переключается и закрывает старый транспорт.
func (c *Conn) Migrate(ctx context.Context, laddr *net.UDPAddr) error {
	pc, err := c.listenLike(laddr)
	if err != nil {
		return err
	}
	setUDPBuffers(pc)
	keepOutside(pc, c.keep)
	newTr := &quic.Transport{Conn: pc}

	path, err := c.qc.AddPath(newTr)
	if err != nil {
		newTr.Close()
		return err
	}
	// Неудачную попытку нельзя убирать через Transport.Close: путь уже принадлежит
	// сессии, и закрытие транспорта рвёт её целиком — то есть провал переезда сам
	// убивал туннель, который переезжал. Держим сокет до конца сессии, как и старые.
	if err := path.Probe(ctx); err != nil {
		path.Close()
		c.park(newTr, pc)
		return err
	}
	if err := path.Switch(); err != nil {
		path.Close()
		c.park(newTr, pc)
		return err
	}

	c.mu.Lock()
	c.prev = append(c.prev, transportSocket{tr: c.tr, pc: c.pc})
	c.tr, c.pc = newTr, pc
	c.mu.Unlock()

	// ВНИМАНИЕ: старый транспорт НЕ закрываем здесь — Transport.Close() рвёт все
	// свои соединения, включая нашу (только что мигрировавшую) сессию. Держим его
	// в c.prev, пока жива Conn.
	// TODO(quicdiver): grace-освобождение старых путей (ретайр connID + close по
	// таймеру), иначе при частой миграции на мобильном копятся сокеты (arch4).
	return nil
}

// Close закрывает сессию и все транспорты (активный + оставшиеся от миграций).
func (c *Conn) Close() error {
	err := c.qc.CloseWithError(0, "")
	c.mu.Lock()
	tr := c.tr
	prev := c.prev
	c.prev = nil
	c.mu.Unlock()
	if tr != nil {
		tr.Close()
	}
	for _, ts := range prev {
		if ts.tr != nil {
			ts.tr.Close()
		}
		if ts.pc != nil {
			ts.pc.Close()
		}
	}
	return err
}

var _ uplink.Conn = (*Conn)(nil)

// Dialer устанавливает Conn до узла.
type Dialer struct {
	// TLS — конфиг клиента. NextProtos дополняется ALPN, если пуст.
	TLS *tls.Config
	// QUIC — конфиг сессии. nil → DefaultConfig.
	QUIC *quic.Config
	// Keep вызывается для каждого созданного сокета. На Android без этого
	// туннель уходит сам в себя: система заворачивает в VPN и его собственный
	// трафик, если сокет не помечен как исключённый.
	Keep func(fd uintptr)
}

// Dial резолвит endpoint (host:port по домену — arch3) и устанавливает сессию.
func (d Dialer) Dial(ctx context.Context, endpoint string) (uplink.Conn, error) {
	addrs, err := resolve(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	if len(addrs) == 1 {
		conn, err := d.reach(ctx, addrs[0])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", addrs[0], err)
		}
		return conn, nil
	}

	round, stop := context.WithCancel(ctx)
	defer stop()

	type finish struct {
		conn uplink.Conn
		err  error
	}
	line := make(chan finish, len(addrs))

	for i, raddr := range addrs {
		go func(n int, where *net.UDPAddr) {
			if n > 0 {
				select {
				case <-time.After(time.Duration(n) * headStart):
				case <-round.Done():
					line <- finish{err: round.Err()}
					return
				}
			}
			conn, err := d.reach(round, where)
			if err != nil {
				line <- finish{err: fmt.Errorf("%s: %w", where, err)}
				return
			}
			line <- finish{conn: conn}
		}(i, raddr)
	}

	tried := make([]string, 0, len(addrs))
	for range addrs {
		got := <-line
		if got.err == nil {
			return got.conn, nil
		}
		tried = append(tried, got.err.Error())
	}
	return nil, errors.New(strings.Join(tried, " / "))
}

func (d Dialer) reach(ctx context.Context, raddr *net.UDPAddr) (uplink.Conn, error) {
	pc, err := listenFor(raddr)
	if err != nil {
		fmt.Printf("dial     %s: no socket: %v\n", raddr, err)
		return nil, err
	}
	setUDPBuffers(pc)
	keepOutside(pc, d.Keep)
	fmt.Printf("dial     %s from %s\n", raddr, pc.LocalAddr())
	tr := &quic.Transport{Conn: pc}

	began := time.Now()
	qc, err := tr.Dial(ctx, raddr, ensureALPN(d.TLS), configOrDefault(d.QUIC))
	if err != nil {
		fmt.Printf("dial     %s gave up after %d ms: %v\n", raddr, time.Since(began).Milliseconds(), err)
		tr.Close()
		return nil, err
	}
	fmt.Printf("dial     %s answered in %d ms\n", raddr, time.Since(began).Milliseconds())
	c := &Conn{qc: qc, tr: tr, pc: pc, remote: raddr, keep: d.Keep}
	c.maxDgram.Store(defaultMaxDatagram)
	return c, nil
}

const headStart = 250 * time.Millisecond

var _ uplink.Dialer = Dialer{}

func DialPacketConn(ctx context.Context, pc net.PacketConn, raddr net.Addr, tlsConf *tls.Config, quicConf *quic.Config) (*Conn, error) {
	tr := &quic.Transport{Conn: pc}
	qc, err := tr.Dial(ctx, raddr, ensureALPN(tlsConf), configOrDefault(quicConf))
	if err != nil {
		tr.Close()
		return nil, err
	}
	c := &Conn{qc: qc, tr: tr, pc: pc, remote: raddr}
	c.maxDgram.Store(defaultMaxDatagram)
	return c, nil
}

// DefaultConfig — базовый quic.Config для QUIC Diver.
//
// Окна — чуть выше BDP и НЕ больше: BDP пути ≈ 768 Мбит × 14 мс ≈ 1.3 МБ.
// Раздутые окна (пробовали 32/64 МБ) разрешают держать в полёте десятки
// мегабайт — они встают в очередь на пути, и это классический bufferbloat:
// замерено RTT под нагрузкой p95 3.4 с (против 32 мс) и throughput 117 Мбит
// (против 560). Стартовое окно чуть больше дефолтных 512 КБ, чтобы не ждать
// авто-тюнинг, потолок оставляем близким к дефолту quic-go.
func DefaultConfig() *quic.Config {
	return &quic.Config{
		EnableDatagrams: true,
		// Смена сети занимает больше, чем прежние 30 секунд: пока роутер поднимает
		// PPPoE, соединению нужно просто дожить до нового пути, иначе переезжать
		// будет нечему и туннель придётся набирать заново.
		MaxIdleTimeout:                 90 * time.Second,
		KeepAlivePeriod:                15 * time.Second,
		InitialStreamReceiveWindow:     2 << 20, // ~1.5x BDP
		MaxStreamReceiveWindow:         6 << 20, // дефолт quic-go
		InitialConnectionReceiveWindow: 3 << 20,
		MaxConnectionReceiveWindow:     15 << 20, // дефолт quic-go
	}
}

func configOrDefault(c *quic.Config) *quic.Config {
	if c == nil {
		return DefaultConfig()
	}
	return c
}

func ensureALPN(t *tls.Config) *tls.Config {
	if t == nil {
		t = &tls.Config{}
	} else {
		t = t.Clone()
	}
	if len(t.NextProtos) == 0 {
		t.NextProtos = []string{ALPN}
	}
	return t
}

// park держит сокет неудавшегося пути, пока сессия может на него сослаться, и
// отпускает потом. Закрыть сразу нельзя — Transport.Close рвёт сессию целиком;
// держать вечно тоже нельзя: на мобильной сети переезды идут пачками.
func (c *Conn) park(tr *quic.Transport, pc *net.UDPConn) {
	c.mu.Lock()
	c.prev = append(c.prev, transportSocket{tr: tr, pc: pc})

	stale := []transportSocket{}
	if over := len(c.prev) - pathKeep; over > 0 {
		stale = append(stale, c.prev[:over]...)
		c.prev = append([]transportSocket{}, c.prev[over:]...)
	}
	c.mu.Unlock()

	for _, one := range stale {
		if one.tr != nil {
			one.tr.Close()
		}
		if one.pc != nil {
			one.pc.Close()
		}
	}
}

// pathKeep — сколько отработавших путей держим. Закрыть путь сразу нельзя:
// Transport.Close рвёт сессию, которая на него ссылалась. Держать все тоже
// нельзя — на мобильной сети переезды идут пачками, и сокеты копятся.
const pathKeep = 6

func keepOutside(pc *net.UDPConn, keep func(fd uintptr)) {
	if keep == nil {
		return
	}
	raw, err := pc.SyscallConn()
	if err != nil {
		return
	}
	raw.Control(keep)
}

// resolve помнит адреса, по которым узел однажды ответил. Имя резолвится
// системой, а система бывает недоступна ровно тогда, когда она нужнее всего:
// на мобильной сети под белым списком оператора DNS не выпускают наружу, и
// клиент, переехавший с Wi-Fi на соту, переставал находить собственный узел —
// хотя сам узел оставался достижим.
var known sync.Map

func Addrs(ctx context.Context, endpoint string) ([]string, error) {
	held, err := resolve(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(held))
	for _, one := range held {
		out = append(out, one.String())
	}
	return out, nil
}

func resolve(ctx context.Context, endpoint string) ([]*net.UDPAddr, error) {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, err
	}
	number, err := net.LookupPort("udp", port)
	if err != nil {
		return nil, err
	}

	if ip, err := netip.ParseAddr(host); err == nil {
		return []*net.UDPAddr{{IP: ip.AsSlice(), Port: number}}, nil
	}

	look, stop := context.WithTimeout(ctx, resolveWait)
	ips, err := net.DefaultResolver.LookupIP(look, "ip", host)
	stop()
	if err != nil || len(ips) == 0 {
		if held, ok := known.Load(endpoint); ok {
			return held.([]*net.UDPAddr), nil
		}
		if err == nil {
			err = fmt.Errorf("no address for %s", host)
		}
		return nil, err
	}

	out := order(ips, number)
	known.Store(endpoint, out)
	fmt.Printf("resolve  %s -> %v (v6 route %v)\n", endpoint, out, holdsV6())
	return out, nil
}

func order(ips []net.IP, port int) []*net.UDPAddr {
	v6 := holdsV6()

	first := make([]*net.UDPAddr, 0, len(ips))
	rest := make([]*net.UDPAddr, 0, len(ips))
	for _, ip := range ips {
		one := &net.UDPAddr{IP: ip, Port: port}
		if ip.To4() != nil || v6 {
			first = append(first, one)
			continue
		}
		rest = append(rest, one)
	}
	return append(first, rest...)
}

func holdsV6() bool {
	c, err := net.Dial("udp6", "[2001:4860:4860::8888]:53")
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func listenFor(raddr *net.UDPAddr) (*net.UDPConn, error) {
	network := "udp6"
	if raddr.IP.To4() != nil {
		network = "udp4"
	}
	return net.ListenUDP(network, &net.UDPAddr{Port: 0})
}

func (c *Conn) listenLike(laddr *net.UDPAddr) (*net.UDPConn, error) {
	network := "udp"
	if remote, ok := c.remote.(*net.UDPAddr); ok && remote != nil {
		if remote.IP.To4() != nil {
			network = "udp4"
		} else {
			network = "udp6"
		}
	}
	if laddr == nil {
		laddr = &net.UDPAddr{Port: 0}
	}
	return net.ListenUDP(network, laddr)
}

const resolveWait = 4 * time.Second
