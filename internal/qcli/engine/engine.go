// Package engine — модель обработки трафика клиента (data-path).
//
// Engine — единственная точка развилки A/B. Под ним всё общее (packet.Source,
// PacketTunnel). Смена модели = смена реализации Engine.
//
//	Модель B (connectip): passthrough — сырые IP из Source заворачиваются в
//	   connect-ip (PacketTunnel), ответы реинжектятся в стек ОС. gVisor на клиенте
//	   нет. Выбрана как основная.
//	Модель A (позже): терминация TCP/UDP в gVisor на клиенте, per-flow потоки.
//	   Отдельная реализация Engine поверх тех же Source и транспорта.
package engine

import (
	"context"

	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
)

// PacketTunnel — двусторонний канал сырых IP-пакетов до узла (клиентская
// сторона). cip.Client удовлетворяет: WritePacket отправляет IP-пакет в туннель
// (и может вернуть готовый ICMP при oversize), ReadPacket читает ответный IP.
type PacketTunnel interface {
	WritePacket(b []byte) (icmp []byte, err error)
	ReadPacket(b []byte) (int, error)
}

// Rewriter мутирует пакет на границе туннеля (на месте). В модели B это
// клиентский NAT: Outbound переписывает src real→assigned перед отправкой,
// Inbound — dst assigned→real после приёма (connect-ip требует назначенный src).
// nil-Rewriter означает отсутствие подмены.
type Rewriter interface {
	Outbound(pkt []byte)
	Inbound(pkt []byte)
}

// Engine гоняет трафик между локальным источником пакетов и туннелем до узла.
type Engine interface {
	// Run обрабатывает трафик до отмены ctx или фатальной ошибки.
	// Реализация многопоточна по умолчанию (arch6).
	Run(ctx context.Context, src packet.Source, tun PacketTunnel) error
}

// MarkedTunnel — туннель, умеющий пометить пакет выходом (Context ID датаграммы).
type MarkedTunnel interface {
	WritePacketMarked(b []byte, mark uint64) (icmp []byte, err error)
}
