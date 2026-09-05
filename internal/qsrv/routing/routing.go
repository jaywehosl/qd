package routing

import "net/netip"

type Target uint8

const (
	Direct Target = iota
	Chain
	Block
)

type Decision struct {
	Target   Target
	Outbound string
}

type Flow struct {
	Proto            uint8
	SrcIP, DstIP     netip.Addr
	SrcPort, DstPort uint16
	PID              uint32
}

type Router interface {
	Route(f Flow) Decision
}

type Default struct{}

func (Default) Route(Flow) Decision { return Decision{Target: Direct} }

var _ Router = Default{}
