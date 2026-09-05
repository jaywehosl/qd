package netstate

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func fixture() *State {
	return &State{
		Revision: 42,
		Nodes: []Node{
			{ID: 1, Tag: "in-1", Address: "1.2.3.4", Port: 443, Role: RoleIngress, Enable: true},
			{ID: 2, Tag: "in-2", Address: "1.2.3.5", Port: 443, Role: RoleIngress, Enable: true},
			{ID: 3, Tag: "out-1", Address: "5.6.7.8", Port: 443, Role: RoleEgress, Enable: true},
			{ID: 4, Tag: "off", Address: "9.9.9.9", Port: 443, Role: RoleEgress, Enable: false},
		},
		Entrypoints: []Entrypoint{
			{ID: 10, NodeID: 1, Port: 443, Enable: true},
			{ID: 11, NodeID: 1, Port: 8443, Enable: true},
			{ID: 12, NodeID: 1, Port: 9443, Enable: false},
			{ID: 20, NodeID: 2, Port: 443, Enable: true},
			{ID: 30, NodeID: 3, Port: 443, Enable: true},
		},
		Groups: []Group{
			{ID: 100, Tag: "russia-in", AllowExit: true, EntrypointIDs: []int{10, 11}},
			{ID: 200, Tag: "cheap", AllowExit: false, EntrypointIDs: []int{20}},
		},
		Clients: []Client{
			{ID: 1000, Tag: "vasya", UUID: "uuid-vasya", GroupID: 100, Enable: true, ExpiryAt: 1735689600000},
			{ID: 1001, Tag: "petya", UUID: "uuid-petya", GroupID: 200, Enable: true},
			{ID: 1002, Tag: "off", UUID: "uuid-off", GroupID: 100, Enable: false},
			{ID: 1003, Tag: "orphan", UUID: "uuid-orphan", GroupID: 999, Enable: true},
		},
		NetworkKey: "network-key",
	}
}

func mustProject(t *testing.T, id int, s *State) *NodeConfig {
	t.Helper()
	cfg, err := Project(id, s)
	if err != nil {
		t.Fatalf("Project(%d): %v", id, err)
	}
	return cfg
}

func TestEgressNeverLearnsAnyClient(t *testing.T) {
	s := fixture()
	for _, n := range s.Nodes {
		if n.Role != RoleEgress || !n.Enable {
			continue
		}
		blob, err := json.Marshal(mustProject(t, n.ID, s))
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range s.Clients {
			if strings.Contains(string(blob), c.UUID) {
				t.Fatalf("egress node %d was given client uuid %q:\n%s", n.ID, c.UUID, blob)
			}
		}
		if strings.Contains(string(blob), `"clients"`) {
			t.Fatalf("egress node %d carries a clients key at all:\n%s", n.ID, blob)
		}
	}
}

func TestNoProjectionCarriesAStrangerKey(t *testing.T) {
	s := fixture()
	for _, n := range s.Nodes {
		if !n.Enable {
			continue
		}
		cfg := mustProject(t, n.ID, s)

		allowed := map[string]bool{}
		for _, p := range s.Nodes {
			if p.Enable && p.Role != n.Role {
				allowed[p.Address] = true
			}
		}

		blob, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range s.Nodes {
			if p.ID == n.ID || p.Address == "" || allowed[p.Address] {
				continue
			}
			if strings.Contains(string(blob), p.Address) {
				t.Fatalf("node %d (%s) carries the address of node %d (%s):\n%s",
					n.ID, n.Role, p.ID, p.Role, blob)
			}
		}
	}
}

func TestDisabledNodeIsNotProjectable(t *testing.T) {
	if _, err := Project(4, fixture()); !errors.Is(err, ErrNodeOff) {
		t.Fatalf("want ErrNodeOff, got %v", err)
	}
	if _, err := Project(777, fixture()); !errors.Is(err, ErrNoSuchNode) {
		t.Fatalf("want ErrNoSuchNode, got %v", err)
	}
}

func TestIngressOnlyLearnsClientsThatCanReachIt(t *testing.T) {
	s := fixture()

	in1 := mustProject(t, 1, s)
	if got := uuids(in1); len(got) != 1 || got[0] != "uuid-vasya" {
		t.Fatalf("node 1 clients = %v, want [uuid-vasya]", got)
	}

	in2 := mustProject(t, 2, s)
	if got := uuids(in2); len(got) != 1 || got[0] != "uuid-petya" {
		t.Fatalf("node 2 clients = %v, want [uuid-petya]", got)
	}
}

func TestDisabledAndOrphanClientsAreDropped(t *testing.T) {
	got := strings.Join(uuids(mustProject(t, 1, fixture())), ",")
	if strings.Contains(got, "uuid-off") {
		t.Fatal("a disabled client was projected")
	}
	if strings.Contains(got, "uuid-orphan") {
		t.Fatal("a client whose group does not exist was projected")
	}
}

func TestAllowExitComesFromTheGroup(t *testing.T) {
	in1 := mustProject(t, 1, fixture())
	if !in1.Clients[0].AllowExit {
		t.Fatal("vasya is in a group that allows exit, but was projected without it")
	}
	in2 := mustProject(t, 2, fixture())
	if in2.Clients[0].AllowExit {
		t.Fatal("petya's group forbids exit, but the node was told otherwise")
	}
}

func TestDisabledEntrypointsAreNotServed(t *testing.T) {
	cfg := mustProject(t, 1, fixture())
	for _, e := range cfg.Ents {
		if e.Port == 9443 {
			t.Fatal("a disabled entrypoint was projected")
		}
	}
	if len(cfg.Ents) != 2 {
		t.Fatalf("want 2 entrypoints, got %d", len(cfg.Ents))
	}
}

func TestEncapOverheadCountsTheHopsActuallyPossible(t *testing.T) {
	s := fixture()
	if got := mustProject(t, 1, s).Self.EncapOverhead; got != 2*EncapLen {
		t.Fatalf("chaining ingress overhead = %d, want %d", got, 2*EncapLen)
	}
	if got := mustProject(t, 3, s).Self.EncapOverhead; got != EncapLen {
		t.Fatalf("egress overhead = %d, want %d", got, EncapLen)
	}

	for i := range s.Nodes {
		if s.Nodes[i].Role == RoleEgress {
			s.Nodes[i].Enable = false
		}
	}
	if got := mustProject(t, 1, s).Self.EncapOverhead; got != EncapLen {
		t.Fatalf("exit-less ingress overhead = %d, want %d", got, EncapLen)
	}
}

func TestPeersCarryAddressOnlyWhereItIsDialled(t *testing.T) {
	s := fixture()

	for _, p := range mustProject(t, 1, s).Peers {
		if p.Role != RoleEgress {
			t.Fatalf("ingress was given a %s peer", p.Role)
		}
		if p.Address == "" || p.Port == 0 {
			t.Fatal("ingress must know where to reach its exit")
		}
	}
	for _, p := range mustProject(t, 3, s).Peers {
		if p.Role != RoleIngress {
			t.Fatalf("egress was given a %s peer", p.Role)
		}
		if p.Address != "" || p.Port != 0 {
			t.Fatalf("egress was given a dialable address for node %d", p.NodeID)
		}
	}
}

func TestDisabledPeerIsNotOffered(t *testing.T) {
	for _, p := range mustProject(t, 1, fixture()).Peers {
		if p.NodeID == 4 {
			t.Fatal("a disabled exit node was offered as a peer")
		}
	}
}

func TestProjectionIsByteStable(t *testing.T) {
	for _, id := range []int{1, 2, 3} {
		var first []byte
		for i := 0; i < 8; i++ {
			blob, err := json.Marshal(mustProject(t, id, fixture()))
			if err != nil {
				t.Fatal(err)
			}
			if first == nil {
				first = blob
				continue
			}
			if string(blob) != string(first) {
				t.Fatalf("node %d projection is not stable:\n%s\n%s", id, first, blob)
			}
		}
	}
}

func TestInputOrderDoesNotChangeOutput(t *testing.T) {
	a, err := json.Marshal(mustProject(t, 1, fixture()))
	if err != nil {
		t.Fatal(err)
	}

	s := fixture()
	reverse(s.Nodes)
	reverse(s.Entrypoints)
	reverse(s.Clients)
	reverse(s.Groups)
	reverse([]string{s.NetworkKey})

	b, err := json.Marshal(mustProject(t, 1, s))
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("row order changed the projection:\n%s\n%s", a, b)
	}
}

func TestNetworkKeyReachesEveryRole(t *testing.T) {
	for _, id := range []int{1, 3} {
		cfg := mustProject(t, id, fixture())
		if len(cfg.Admins) != 1 || cfg.Admins[0] != "network-key" {
			t.Fatalf("node %d got %v, want the one network key", id, cfg.Admins)
		}
	}
}

func uuids(c *NodeConfig) []string {
	out := make([]string, 0, len(c.Clients))
	for _, x := range c.Clients {
		out = append(out, x.UUID)
	}
	return out
}

func reverse[T any](s []T) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
