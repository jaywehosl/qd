// Package routing — маршрутизация флоу: напрямую в интернет, в chain или блок.
//
// Правила приходят из БД узла (arch: правила и аутбаунды хранятся в SQLite).
// Решение принимается по 5-tuple (+PID, если известен). В модели B решение обычно
// принимает узел (он видит dst из IP); интерфейс общий и для клиентской модели A,
// где роутинг возможен уже на клиенте.
package routing

import "net/netip"

// Target — куда направить флоу.
type Target uint8

const (
	// Direct — в интернет через сетевой интерфейс узла.
	Direct Target = iota
	// Chain — в upstream-узел (chain proxy) по аутбаунду Decision.Outbound.
	Chain
	// Block — отбросить.
	Block
)

// Decision — результат маршрутизации.
type Decision struct {
	Target   Target
	Outbound string // ID аутбаунда в цепочке; значим при Target==Chain.
}

// Flow — ключ маршрутизации (5-tuple + процесс).
type Flow struct {
	Proto            uint8
	SrcIP, DstIP     netip.Addr
	SrcPort, DstPort uint16
	PID              uint32
}

// Router выбирает направление для флоу по загруженным правилам.
type Router interface {
	Route(f Flow) Decision
}

// Default — роутер по умолчанию: всё напрямую. Точка старта до загрузки правил из БД.
type Default struct{}

func (Default) Route(Flow) Decision { return Decision{Target: Direct} }

var _ Router = Default{}
