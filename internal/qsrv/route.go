package qsrv

const (
	HeaderRoute = "Qd-Route"
	HeaderToken = "Qd-Token"
	HeaderHops  = "Qd-Hops"
	HeaderNode  = "Qd-Node"
	HeaderSeat  = "Qd-Seat"
	HeaderProto = "Qd-Proto"

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

func (n *Node) pickPeer(tag string) Peer {
	peers := n.peers()
	for _, p := range peers {
		if p.ID == tag || p.Tag == tag {
			return p
		}
	}
	if tag != AnyExit || len(peers) == 0 {
		return Peer{}
	}
	return peers[int(n.turn.Add(1))%len(peers)]
}

func settled(route string) string {
	if route == HereExit {
		return ""
	}
	return route
}
