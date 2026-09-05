// Package uplink — живая транспортная сессия клиент↔узел поверх QUIC/HTTP3.
//
// Одна Conn мультиплексирует весь трафик клиента. Реализация обязана переживать
// смену сети (arch4): при смене пути (Wi-Fi↔LTE, пересборка PPPoE) сессия НЕ
// рвётся — QUIC connection ID сохраняется, сокет ре-биндится, путь валидируется,
// проксирование продолжается без «передозвона».
//
// Conn даёт оба примитива, чтобы обе модели engine влезли без переделки:
//   - датаграммы (RFC 9221) — модель B несёт ими IP-пакеты (connect-ip),
//     а также UDP-флоу будущей модели A (connect-udp);
//   - потоки — будущая модель A (connect-tcp) и fallback для крупных payload,
//     не влезающих в датаграмму.
package uplink

import (
	"context"
	"io"
)

// Conn — установленная QUIC/H3-сессия до одного узла.
type Conn interface {
	// SendDatagram шлёт ненадёжную датаграмму. Потеря допустима (как в реальной
	// сети): надёжность обеспечивает end-to-end протокол внутри туннелируемого IP.
	SendDatagram(b []byte) error

	// RecvDatagram принимает ненадёжную датаграмму от узла.
	RecvDatagram(ctx context.Context) ([]byte, error)

	// OpenStream открывает надёжный двунаправленный поток.
	OpenStream(ctx context.Context) (Stream, error)

	// MaxDatagramSize — текущий лимит полезной датаграммы в байтах.
	// Нужен для MTU-инженерии модели B (MSS-clamp, порог stream-fallback).
	MaxDatagramSize() int

	Close() error
}

// Stream — надёжный двунаправленный поток внутри Conn.
type Stream interface {
	io.ReadWriteCloser
}

// Dialer устанавливает Conn до узла.
//
// endpoint — доменное имя (не IP): клиент всегда резолвит имя и переустанавливает
// сессию по нему. Это выполняет arch3 — после миграции узла на новую VM (сменилась
// A-запись) клиент переподключается по тому же домену, ничего не меняя у себя.
type Dialer interface {
	Dial(ctx context.Context, endpoint string) (Conn, error)
}
