package netstate

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func fixture() *State {
	return &State{
		Nodes: []Node{
			{ID: 1, Tag: "in-1", Address: "198.51.100.10", Port: 443, Role: RoleIngress, Enable: true},
			{ID: 2, Tag: "in-2", Address: "198.51.100.11", Port: 443, Role: RoleIngress, Enable: true},
			{ID: 3, Tag: "out-1", Address: "198.51.100.20", Port: 443, Role: RoleEgress, Enable: true},
			{ID: 4, Tag: "off", Address: "198.51.100.21", Port: 443, Role: RoleEgress, Enable: false},
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
		if got := mustProject(t, n.ID, s).Clients; len(got) != 0 {
			t.Fatalf("egress node %d was given clients %v", n.ID, got)
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

func TestPeersAreTheOppositeRole(t *testing.T) {
	s := fixture()
	for _, p := range mustProject(t, 1, s).Peers {
		if p.Role != RoleEgress {
			t.Fatalf("ingress was given a %s peer", p.Role)
		}
	}
	for _, p := range mustProject(t, 3, s).Peers {
		if p.Role != RoleIngress {
			t.Fatalf("egress was given a %s peer", p.Role)
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

	b, err := json.Marshal(mustProject(t, 1, s))
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("row order changed the projection:\n%s\n%s", a, b)
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
