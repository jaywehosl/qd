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
	"net"
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
	pc, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return err
	}
	setUDPBuffers(pc)
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
	raddr, err := resolve(endpoint)
	if err != nil {
		return nil, err
	}
	pc, err := net.ListenUDP("udp", &net.UDPAddr{Port: 0})
	if err != nil {
		return nil, err
	}
	setUDPBuffers(pc)
	keepOutside(pc, d.Keep)
	tr := &quic.Transport{Conn: pc}

	qc, err := tr.Dial(ctx, raddr, ensureALPN(d.TLS), configOrDefault(d.QUIC))
	if err != nil {
		tr.Close()
		return nil, err
	}
	c := &Conn{qc: qc, tr: tr, pc: pc, remote: raddr}
	c.maxDgram.Store(defaultMaxDatagram)
	return c, nil
}

var _ uplink.Dialer = Dialer{}

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

// resolve помнит адрес, по которому узел однажды ответил. Имя резолвится
// системой, а система бывает недоступна ровно тогда, когда она нужнее всего:
// на мобильной сети под белым списком оператора DNS не выпускают наружу, и
// клиент, переехавший с Wi-Fi на соту, переставал находить собственный узел —
// хотя сам узел оставался достижим.
var known sync.Map

func resolve(endpoint string) (*net.UDPAddr, error) {
	addr, err := net.ResolveUDPAddr("udp", endpoint)
	if err == nil {
		known.Store(endpoint, addr)
		return addr, nil
	}
	if held, ok := known.Load(endpoint); ok {
		return held.(*net.UDPAddr), nil
	}
	return nil, err
}
