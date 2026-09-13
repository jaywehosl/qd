package clientapi

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink/relay"
)

type Entrypoint struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
}

type Standing struct {
	Known          bool         `json:"known"`
	Carried        bool         `json:"carried"`
	Enable         bool         `json:"enable"`
	Expired        bool         `json:"expired"`
	Tag            string       `json:"tag"`
	AllowExit      bool         `json:"allowExit"`
	RefreshMinutes int          `json:"refreshMinutes"`
	Denied         string       `json:"refused"`
	Entrypoints    []Entrypoint `json:"entrypoints"`
	Relays         []relay.Link `json:"relays"`
	Admin          bool         `json:"admin"`
	FixedRate      int          `json:"fixedRate"`
	Peers          []string     `json:"peers"`
}

func (s Standing) Refused() bool { return !s.Carried && s.Why() != "" }

func (s Standing) Why() string {
	switch {
	case !s.Known:
		return "This subscription is no longer valid — ask the administrator for a new link."
	case !s.Enable:
		return "This client has been disabled by the administrator."
	case s.Expired:
		return "This subscription has expired."
	case s.Denied != "":
		return s.Denied
	}
	return ""
}

func claim(me Device, token string) map[string]any {
	return map[string]any{
		"token": token, "device": me.ID, "platform": me.Platform,
		"model": me.Model, "kind": me.Kind, "name": me.Name,
	}
}

type Asker interface {
	Ask(endpoint, op, auth string, body any, out any) error
}

func Announce(op string, nodes []clientstate.Node, token string, me Device, wire Asker) int {
	if wire == nil || token == "" {
		return 0
	}

	body := claim(me, token)
	var heard atomic.Int32
	var wg sync.WaitGroup
	for _, n := range nodes {
		wg.Add(1)
		go func(where string) {
			defer wg.Done()
			if err := wire.Ask(where, op, token, body, nil); err == nil {
				heard.Add(1)
			}
		}(n.Endpoint())
	}
	wg.Wait()
	return int(heard.Load())
}

func (a *API) sweep() (int, Standing) {
	sub, err := a.db.Subscription()
	if err != nil || !sub.Imported {
		return 0, Standing{}
	}
	nodes, err := a.db.Nodes()
	wire := a.platform.Wire()
	if err != nil || wire == nil || len(nodes) == 0 {
		return 0, Standing{}
	}

	type result struct {
		id       int
		latency  int
		standing Standing
	}
	results := make(chan result, len(nodes))
	body := claim(a.platform.Identify(), sub.Key)

	for _, n := range nodes {
		go func(n clientstate.Node) {
			var answer Standing
			began := time.Now()
			if err := wire.Ask(n.Endpoint(), "whoami", sub.Key, body, &answer); err != nil {
				results <- result{id: n.ID, latency: -1}
				return
			}
			results <- result{n.ID, int(time.Since(began).Milliseconds()), answer}
		}(n)
	}

	reached := 0
	var best Standing
	answered := make(map[int]bool, len(nodes))
	deadline := time.After(sweepWait)
collect:
	for range nodes {
		select {
		case r := <-results:
			answered[r.id] = true
			a.db.MarkReach(r.id, r.latency, r.latency >= 0)
			if r.latency < 0 {
				continue
			}
			if reached == 0 || (r.standing.Known && !best.Known) {
				best = r.standing
			}
			reached++
		case <-deadline:
			break collect
		}
	}
	for _, n := range nodes {
		if !answered[n.ID] {
			a.db.MarkReach(n.ID, -1, false)
		}
	}
	return reached, best
}

const sweepWait = 4 * time.Second

func (a *API) take() (int, error) {
	reached, answer := a.sweep()
	if reached == 0 {
		return 0, nil
	}
	a.adoptNetworkDefaults(answer)
	if answer.Refused() {
		a.platform.Stop()
		a.db.Notify("error", answer.Why(), time.Now().UnixMilli())
		return reached, errors.New(answer.Why())
	}
	return reached, nil
}
