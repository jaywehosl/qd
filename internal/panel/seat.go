package panel

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/localapi"
	"github.com/jaywehosl/qd/internal/qdcrypt"
	"github.com/jaywehosl/qd/internal/qwire"
)

const seatIdle = 2 * time.Minute

type Seat struct {
	db   *clientstate.DB
	wire *qwire.Dialer

	mu      sync.Mutex
	fleet   *Fleet
	api     *API
	mux     *http.ServeMux
	stop    chan struct{}
	entered bool
	touched time.Time

	finding sync.Mutex
}

func NewSeat(key *qdcrypt.Key, db *clientstate.DB, wire *qwire.Dialer) *Seat {
	s := &Seat{db: db, wire: wire}
	s.SetKey(key)
	return s
}

func (s *Seat) opened() {
	s.mu.Lock()
	s.touched = time.Now()
	s.entered = true
	s.mu.Unlock()

	s.discover()
}

func (s *Seat) open() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.entered && time.Since(s.touched) < seatIdle
}

func (s *Seat) SetKey(key *qdcrypt.Key) {
	if key == nil {
		return
	}

	fleet := NewFleet(*key, s.wire)
	mux := http.NewServeMux()
	api := NewAPI(fleet, s.db)
	api.Routes(mux)

	stop := make(chan struct{})

	s.mu.Lock()
	was := s.stop
	s.fleet, s.api, s.mux, s.stop = fleet, api, mux, stop
	s.mu.Unlock()

	if was != nil {
		close(was)
	}
	go s.discover()
	go api.Converge(stop, s.open)
}

func (s *Seat) Feed() []localapi.Push {
	s.mu.Lock()
	api := s.api
	s.mu.Unlock()
	if api == nil {
		return nil
	}

	out := []localapi.Push{}
	for name, payload := range api.Live() {
		out = append(out, localapi.Push{Type: name, Payload: payload})
	}
	return out
}

func (s *Seat) handler() (*Fleet, *http.ServeMux) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fleet, s.mux
}

func (s *Seat) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.opened()
	_, mux := s.handler()
	if mux == nil {
		http.NotFound(w, r)
		return
	}
	mux.ServeHTTP(w, r)
}

func (s *Seat) discover() {
	s.finding.Lock()
	defer s.finding.Unlock()

	fleet, _ := s.handler()
	if fleet == nil || len(fleet.Nodes()) > 0 {
		return
	}

	sub, err := s.db.Subscription()
	if err != nil || !sub.Imported {
		return
	}
	token := sub.Key
	fleet.SetToken(token)
	fleet.SetTag(sub.Tag)

	nodes, err := s.db.Nodes()
	if err != nil || len(nodes) == 0 {
		return
	}

	for _, n := range nodes {
		seed := NodeAddress{ID: n.ID, Tag: n.Name, Address: n.Address, Port: n.Port}
		if err := fleet.Discover(seed); err != nil {
			continue
		}

		admin := fleet.IsAdmin(seed, token)
		if admin != sub.Admin {
			s.db.SetAdmin(admin)
		}
		if admin {
			fmt.Printf("admin    %s recognized this key, the panel is open\n", n.Name)
		}
		return
	}
	fmt.Printf("admin    no node answered the control channel yet\n")
}

func (s *Seat) Peers() []string {
	fleet, _ := s.handler()
	if fleet == nil {
		return nil
	}
	out := []string{}
	for _, n := range fleet.Nodes() {
		out = append(out, n.Address)
	}
	return out
}
