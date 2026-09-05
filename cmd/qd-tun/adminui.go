//go:build windows

package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/localapi"
	"github.com/jaywehosl/quic-diver/internal/panel"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
)

type adminUI struct {
	prefs panel.Prefs
	db    *clientstate.DB

	mu      sync.Mutex
	fleet   *panel.Fleet
	api     *panel.API
	mux     *http.ServeMux
	stop    chan struct{}
	entered bool
	touched time.Time
}

func newAdminUI(key *qdcrypt.Key, db *clientstate.DB) *adminUI {
	a := &adminUI{prefs: db, db: db}
	a.SetKey(key)
	return a
}

func (a *adminUI) opened() {
	a.mu.Lock()
	a.touched = time.Now()
	a.entered = true
	fleet, db := a.fleet, a.db
	a.mu.Unlock()

	if fleet == nil || len(fleet.Nodes()) > 0 {
		return
	}
	a.discover(db)
}

func (a *adminUI) open() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.entered && time.Since(a.touched) < panelIdle
}

const panelIdle = 2 * time.Minute

// SetKey rebuilds the fleet when the key arrives. On a first run there is no
// key until a link is imported, so the panel has to be able to come to life
// without a restart.
func (a *adminUI) SetKey(key *qdcrypt.Key) {
	if key == nil {
		return
	}

	fleet := panel.NewFleet(*key, nodeTalk)
	mux := http.NewServeMux()
	api := panel.NewAPI(fleet, a.prefs)
	api.Routes(mux)

	stop := make(chan struct{})

	a.mu.Lock()
	was := a.stop
	a.fleet, a.api, a.mux, a.stop = fleet, api, mux, stop
	a.mu.Unlock()

	if was != nil {
		close(was)
	}
	if a.open() {
		go a.discover(a.db)
	}
	go api.Converge(stop, a.open)
}

// live is what the open panel gets pushed instead of polling for it.
func (a *adminUI) live() []localapi.Push {
	a.mu.Lock()
	api := a.api
	a.mu.Unlock()
	if api == nil {
		return nil
	}

	out := []localapi.Push{}
	for name, payload := range api.Live() {
		out = append(out, localapi.Push{Type: name, Payload: payload})
	}
	return out
}

func (a *adminUI) handler() (*panel.Fleet, *http.ServeMux) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.fleet, a.mux
}

func (a *adminUI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.opened()
	_, mux := a.handler()
	if mux == nil {
		http.NotFound(w, r)
		return
	}
	mux.ServeHTTP(w, r)
}

// discover seeds the fleet from the subscription: the link names one node, and
// that node's database names the rest. It also asks whether this key
// administers anything — the node decides that, not the link.
func (a *adminUI) discover(db *clientstate.DB) {
	fleet, _ := a.handler()
	if fleet == nil {
		return
	}

	sub, err := db.Subscription()
	if err != nil || !sub.Imported {
		return
	}
	// The client half of the link is the identity: the node looks it up in its
	// clients table and answers from that row. The network key only seals the
	// transport — everyone in the network holds it, so it proves nothing about
	// who may administer.
	token := sub.Key
	fleet.SetToken(token)
	fleet.SetTag(sub.Tag)

	nodes, err := db.Nodes()
	if err != nil || len(nodes) == 0 {
		return
	}

	for _, n := range nodes {
		seed := panel.NodeAddress{ID: n.ID, Tag: n.Name, Address: n.Address, Port: n.Port}
		if err := fleet.Discover(seed); err != nil {
			continue
		}

		admin := fleet.IsAdmin(seed, token)
		if admin != sub.Admin {
			sub.Admin = admin
			db.SaveSubscription(sub)
		}
		if admin {
			fmt.Printf("admin    %s recognised this key, the panel is open\n", n.Name)
		}
		return
	}
	fmt.Printf("admin    no node answered the control channel yet\n")
}
