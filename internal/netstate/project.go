package netstate

import (
	"sort"

	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
)

// Project builds the configuration for one node.
//
// The two roles are built by separate constructors that each ENUMERATE what
// they include. There is deliberately no shared base that one of them then
// strips: forgetting to add something makes a node miss a feature, forgetting
// to remove something hands an egress the list of every client's key. Only one
// of those two mistakes is recoverable, so the code is shaped to make the other
// one impossible rather than merely unlikely.
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
		return projectIngress(n, s), nil
	case RoleEgress:
		return projectEgress(n, s), nil
	default:
		return nil, ErrUnknownRole
	}
}

func projectIngress(n *Node, s *State) *NodeConfig {
	peers := s.peersFor(RoleEgress, true)
	return &NodeConfig{
		Revision: s.Revision,
		Self: Self{
			NodeID: n.ID,
			Role:   RoleIngress,
			// An ingress that can hand traffic to an egress encapsulates twice
			// on that path. MSS has to be clamped for the longer of the two
			// paths, not the one that happens to be in use.
			EncapOverhead: EncapLen * hops(len(peers) > 0),
		},
		Ents:    s.entrypointsOf(n.ID),
		Clients: s.clientsReaching(n.ID),
		Peers:   peers,
		Admins:  s.adminKeys(),
	}
}

func projectEgress(n *Node, s *State) *NodeConfig {
	return &NodeConfig{
		Revision: s.Revision,
		Self: Self{
			NodeID:        n.ID,
			Role:          RoleEgress,
			EncapOverhead: EncapLen,
		},
		Ents:   s.entrypointsOf(n.ID),
		Peers:  s.peersFor(RoleIngress, false),
		Admins: s.adminKeys(),
		// Clients omitted on purpose — an egress authorises nobody, so it is
		// never told who anybody is.
	}
}

func hops(chained bool) int {
	if chained {
		return 2
	}
	return 1
}

func (s *State) Node(id int) *Node {
	for i := range s.Nodes {
		if s.Nodes[i].ID == id {
			return &s.Nodes[i]
		}
	}
	return nil
}

func (s *State) group(id int) *Group {
	for i := range s.Groups {
		if s.Groups[i].ID == id {
			return &s.Groups[i]
		}
	}
	return nil
}

func (s *State) entrypointsOf(nodeID int) []CfgEnt {
	out := []CfgEnt{}
	for _, e := range s.Entrypoints {
		if e.NodeID == nodeID && e.Enable {
			out = append(out, CfgEnt{Port: e.Port})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	return out
}

// clientsReaching returns only the clients whose group actually contains an
// entrypoint on this node. A client that cannot dial this node has no reason to
// be known to it, and shipping the whole roster to every ingress would make one
// seized node worth the entire client list.
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
		g := s.group(c.GroupID)
		if g == nil {
			continue
		}
		reaches := false
		for _, id := range g.EntrypointIDs {
			if local[id] {
				reaches = true
				break
			}
		}
		if !reaches {
			continue
		}
		out = append(out, CfgClient{
			UUID: c.UUID,
			// Expiry travels rather than being applied here: a client whose
			// time runs out while this revision is still the live one has to be
			// evicted by the node, so the node needs the date either way. That
			// makes filtering at projection time a saving of nothing.
			ExpiryAt:  c.ExpiryAt,
			AllowExit: c.MayExit(g),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UUID < out[j].UUID })
	return out
}

// peersFor lists every enabled node of the given role. Node selection is a race
// in both directions — a client races the entrypoints, an ingress races the
// exits — so every node of the opposite role is a legitimate peer.
func (s *State) peersFor(role Role, withAddress bool) []CfgPeer {
	out := []CfgPeer{}
	for _, n := range s.Nodes {
		if n.Role != role || !n.Enable {
			continue
		}
		p := CfgPeer{NodeID: n.ID, Role: n.Role}
		if n.UUID != "" {
			p.Session = qdcrypt.SessionID(n.UUID)
		}
		if withAddress {
			p.Address, p.Port = n.Address, n.Port
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out
}

func (s *State) adminKeys() []string {
	if s.NetworkKey == "" {
		return []string{}
	}
	return []string{s.NetworkKey}
}
