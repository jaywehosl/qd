package qsrv

const (
	HeaderRoute   = "Qd-Route"
	HeaderToken   = "Qd-Token"
	HeaderAuth    = "Qd-Auth"
	HeaderHops    = "Qd-Hops"
	HeaderNode    = "Qd-Node"
	HeaderSeat    = "Qd-Seat"
	HeaderSession = "Qd-Session"
	HeaderProto   = "Qd-Proto"
	HeaderDevice  = "Qd-Device"
	HeaderAddr    = "Qd-Addr"

	AnyExit  = "egress"
	HereExit = "here"
)

const (
	MarkHere   uint64 = 0
	MarkEgress uint64 = 1
)

type Peer struct {
	ID       string
	Tag      string
	Endpoint string
}

func (n *Node) peers() []Peer {
	if n.cfg.Peers == nil {
		return nil
	}
	return n.cfg.Peers()
}

func settled(route string) string {
	if route == HereExit {
		return ""
	}
	return route
}
