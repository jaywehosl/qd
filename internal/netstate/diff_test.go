package netstate

import (
	"encoding/json"
	"strings"
	"testing"
)

func base() *State {
	return &State{
		Revision: 42,
		Nodes: []Node{
			{ID: 1, Tag: "node-1", Address: "1.2.3.4", Port: 443, Role: RoleIngress, Enable: true},
		},
		Entrypoints: []Entrypoint{
			{ID: 10, NodeID: 1, Port: 443, Enable: true},
			{ID: 11, NodeID: 1, Port: 8443, Enable: true},
		},
		Groups: []Group{
			{ID: 100, Tag: "russia-in", AllowExit: true, EntrypointIDs: []int{10}},
		},
		Clients: []Client{
			{ID: 1000, Tag: "vasya", UUID: "u-vasya", GroupID: 100, Enable: true},
		},
		NetworkKey: strings.Repeat("a", 64),
	}
}

func has(t *testing.T, got []Change, want Change) {
	t.Helper()
	for _, c := range got {
		if c == want {
			return
		}
	}
	t.Fatalf("missing %+v in %+v", want, got)
}

func TestNoEditsMeansNoChanges(t *testing.T) {
	got := Diff(base(), base())
	if len(got) != 0 {
		t.Fatalf("an untouched draft reported %+v", got)
	}
	blob, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(blob) != "[]" {
		t.Fatalf("empty diff encoded as %s", blob)
	}
}

func TestFirstPublishListsEverything(t *testing.T) {
	got := Diff(nil, base())
	has(t, got, Change{"node", "node-1", ActionAdded})
	has(t, got, Change{"entrypoint", "node-1:443", ActionAdded})
	has(t, got, Change{"group", "russia-in", ActionAdded})
	has(t, got, Change{"client", "vasya", ActionAdded})
	if len(got) != 6 {
		t.Fatalf("want 6 additions (2 entrypoints, 1 admin), got %d: %+v", len(got), got)
	}
}

func TestAddRemoveUpdateAreTold_Apart(t *testing.T) {
	cur := base()
	cur.Clients = append(cur.Clients, Client{ID: 1001, Tag: "petya", UUID: "u-petya", GroupID: 100, Enable: true})
	cur.Clients[0].Enable = false
	cur.Nodes = nil

	got := Diff(base(), cur)
	has(t, got, Change{"client", "petya", ActionAdded})
	has(t, got, Change{"client", "vasya", ActionUpdated})
	has(t, got, Change{"node", "node-1", ActionRemoved})
}

func TestGroupMembershipIsItsOwnAction(t *testing.T) {
	cur := base()
	cur.Groups[0].EntrypointIDs = []int{10, 11}

	got := Diff(base(), cur)
	has(t, got, Change{"group", "russia-in", ActionEntrypoints})
	if len(got) != 1 {
		t.Fatalf("a membership edit reported %+v", got)
	}
}

func TestReorderingMembershipIsNotAChange(t *testing.T) {
	old := base()
	old.Groups[0].EntrypointIDs = []int{10, 11}
	cur := base()
	cur.Groups[0].EntrypointIDs = []int{11, 10}

	if got := Diff(old, cur); len(got) != 0 {
		t.Fatalf("row order was reported as an edit: %+v", got)
	}
}

func TestFieldEditOutranksMembership(t *testing.T) {
	cur := base()
	cur.Groups[0].AllowExit = false
	cur.Groups[0].EntrypointIDs = []int{10, 11}

	got := Diff(base(), cur)
	if len(got) != 1 || got[0].Action != ActionUpdated {
		t.Fatalf("want a single 'updated', got %+v", got)
	}
}

func TestAnEditThatNoNodeSeesIsStillAChange(t *testing.T) {
	cur := base()
	cur.Clients[0].Comment = "paid until March"

	if got := Diff(base(), cur); len(got) != 1 {
		t.Fatalf("a comment edit reported %+v", got)
	}
}

func TestEntrypointIsNamedByNodeAndPort(t *testing.T) {
	cur := base()
	cur.Entrypoints[0].Remark = "renamed"

	got := Diff(base(), cur)
	has(t, got, Change{"entrypoint", "node-1:443", ActionUpdated})
}

func TestRenameReadsAsRemovedAndAdded(t *testing.T) {
	cur := base()
	cur.Clients[0].Tag = "vasiliy"

	got := Diff(base(), cur)
	has(t, got, Change{"client", "vasya", ActionRemoved})
	has(t, got, Change{"client", "vasiliy", ActionAdded})
}

func TestNetworkKeyChangeIsReported(t *testing.T) {
	cur := base()
	cur.NetworkKey = strings.Repeat("b", 64)

	got := Diff(base(), cur)
	if len(got) != 1 || got[0].Kind != "network key" || got[0].Action != ActionUpdated {
		t.Fatalf("want one network key change, got %+v", got)
	}
	if n := len([]rune(got[0].Name)); n > 14 {
		t.Fatalf("the whole key was printed (%d runes): %q", n, got[0].Name)
	}
}

func TestDiffOrderIsStable(t *testing.T) {
	cur := base()
	cur.Clients = append(cur.Clients,
		Client{ID: 1001, Tag: "petya", UUID: "u-p", GroupID: 100, Enable: true},
		Client{ID: 1002, Tag: "anna", UUID: "u-a", GroupID: 100, Enable: true})
	cur.NetworkKey = strings.Repeat("c", 64)

	var first string
	for i := 0; i < 12; i++ {
		blob, err := json.Marshal(Diff(base(), cur))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = string(blob)
			continue
		}
		if string(blob) != first {
			t.Fatalf("diff order moved between calls:\n%s\n%s", first, blob)
		}
	}
}
