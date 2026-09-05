package netstate

import (
	"fmt"
	"sort"
)

// Change is one line of "what publishing would do", as the header's draft
// summary shows it.
type Change struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Action string `json:"action"`
}

const (
	ActionAdded       = "added"
	ActionRemoved     = "removed"
	ActionUpdated     = "updated"
	ActionEntrypoints = "entrypoints"
)

// Diff describes the draft against the last published revision.
//
// Named by tag rather than by id: the operator recognises "vasya", not 1000,
// and a summary they cannot read is a summary they will publish without
// reading. A row whose tag changed therefore reads as a removal plus an
// addition, which is what it looks like from the outside anyway.
func Diff(old, cur *State) []Change {
	if old == nil {
		old = &State{}
	}
	if cur == nil {
		cur = &State{}
	}

	var out []Change
	out = append(out, diffNodes(old, cur)...)
	out = append(out, diffEntrypoints(old, cur)...)
	out = append(out, diffGroups(old, cur)...)
	out = append(out, diffClients(old, cur)...)
	out = append(out, diffAdmins(old, cur)...)

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	if out == nil {
		out = []Change{}
	}
	return out
}

func diffNodes(old, cur *State) []Change {
	was := map[string]Node{}
	for _, n := range old.Nodes {
		was[n.Tag] = n
	}
	var out []Change
	for _, n := range cur.Nodes {
		prev, existed := was[n.Tag]
		if !existed {
			out = append(out, Change{"node", n.Tag, ActionAdded})
			continue
		}
		delete(was, n.Tag)
		if prev != n {
			out = append(out, Change{"node", n.Tag, ActionUpdated})
		}
	}
	for tag := range was {
		out = append(out, Change{"node", tag, ActionRemoved})
	}
	return out
}

func diffEntrypoints(old, cur *State) []Change {
	// An entrypoint has no tag of its own, so it is named the way the panel
	// shows it: the node it lives on and the port it answers.
	name := func(s *State, e Entrypoint) string {
		node := "?"
		if n := s.Node(e.NodeID); n != nil {
			node = n.Tag
		}
		return fmt.Sprintf("%s:%d", node, e.Port)
	}

	was := map[string]Entrypoint{}
	for _, e := range old.Entrypoints {
		was[name(old, e)] = e
	}
	var out []Change
	for _, e := range cur.Entrypoints {
		k := name(cur, e)
		prev, existed := was[k]
		if !existed {
			out = append(out, Change{"entrypoint", k, ActionAdded})
			continue
		}
		delete(was, k)
		if prev != e {
			out = append(out, Change{"entrypoint", k, ActionUpdated})
		}
	}
	for k := range was {
		out = append(out, Change{"entrypoint", k, ActionRemoved})
	}
	return out
}

func diffGroups(old, cur *State) []Change {
	was := map[string]Group{}
	for _, g := range old.Groups {
		was[g.Tag] = g
	}
	var out []Change
	for _, g := range cur.Groups {
		prev, existed := was[g.Tag]
		if !existed {
			out = append(out, Change{"group", g.Tag, ActionAdded})
			continue
		}
		delete(was, g.Tag)

		membershipMoved := !sameInts(prev.EntrypointIDs, g.EntrypointIDs)
		fieldsMoved := prev.ID != g.ID || prev.AllowExit != g.AllowExit
		switch {
		case fieldsMoved:
			out = append(out, Change{"group", g.Tag, ActionUpdated})
		case membershipMoved:
			// Called out separately because it is the change that silently
			// rewrites which nodes a whole set of clients can reach.
			out = append(out, Change{"group", g.Tag, ActionEntrypoints})
		}
	}
	for tag := range was {
		out = append(out, Change{"group", tag, ActionRemoved})
	}
	return out
}

func diffClients(old, cur *State) []Change {
	was := map[string]Client{}
	for _, c := range old.Clients {
		was[c.Tag] = c
	}
	var out []Change
	for _, c := range cur.Clients {
		prev, existed := was[c.Tag]
		if !existed {
			out = append(out, Change{"client", c.Tag, ActionAdded})
			continue
		}
		delete(was, c.Tag)
		if prev != c {
			out = append(out, Change{"client", c.Tag, ActionUpdated})
		}
	}
	for tag := range was {
		out = append(out, Change{"client", tag, ActionRemoved})
	}
	return out
}

func diffAdmins(old, cur *State) []Change {
	if old.NetworkKey == cur.NetworkKey {
		return nil
	}
	return []Change{{"network key", short(cur.NetworkKey), ActionUpdated}}
}

// A fingerprint is 43 characters and unreadable in a list. The head of it is
// enough to tell two keys apart, which is all the summary is for.
func short(key string) string {
	if len(key) <= 12 {
		return key
	}
	return key[:12] + "…"
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]int(nil), a...)
	y := append([]int(nil), b...)
	sort.Ints(x)
	sort.Ints(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
