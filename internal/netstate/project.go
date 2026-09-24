package netstate

import (
	"sort"

	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
)

func Project(nodeID int, s *State) (*NodeConfig, error) {
	n := s.Node(nodeID)
	if n == nil {
		return nil, ErrNoSuchNode
	}
	if !n.Enable {
		return nil, ErrNodeOff
	}

	switch n.Role {
	case RoleIngress:
		return &NodeConfig{Clients: s.clientsReaching(n.ID), Peers: s.peersFor(RoleEgress)}, nil
	case RoleEgress:
		return &NodeConfig{Peers: s.peersFor(RoleIngress)}, nil
	default:
		return nil, ErrUnknownRole
	}
}

func (s *State) Node(id int) *Node {
	for i := range s.Nodes {
		if s.Nodes[i].ID == id {
			return &s.Nodes[i]
		}
	}
	return nil
}

func (s *State) Group(id int) *Group {
	for i := range s.Groups {
		if s.Groups[i].ID == id {
			return &s.Groups[i]
		}
	}
	return nil
}

func (s *State) clientsReaching(nodeID int) []CfgClient {
	local := map[int]bool{}
	for _, e := range s.Entrypoints {
		if e.NodeID == nodeID && e.Enable {
			local[e.ID] = true
		}
	}

	out := []CfgClient{}
	for _, c := range s.Clients {
		if !c.Enable || c.UUID == "" {
			continue
		}
		g := s.Group(c.GroupID)
		if g == nil {
			continue
		}
		for _, id := range g.EntrypointIDs {
			if local[id] {
				out = append(out, CfgClient{UUID: c.UUID, ExpiryAt: c.ExpiryAt, AllowExit: c.MayExit(g), RouteDNS: g.RouteDNS})
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UUID < out[j].UUID })
	return out
}

func (s *State) peersFor(role Role) []CfgPeer {
	out := []CfgPeer{}
	for _, n := range s.Nodes {
		if n.Role != role || !n.Enable {
			continue
		}
		p := CfgPeer{NodeID: n.ID, Role: n.Role}
		if n.UUID != "" {
			p.Session = qdcrypt.SessionID(n.UUID)
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out
}
