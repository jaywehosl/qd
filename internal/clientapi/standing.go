package clientapi

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
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

// Asker — способ задать узлу один вопрос. За ним стоит общий QUIC-диалер: свои
// кадры, номера запросов и таблица ожидающих ответов ушли вместе с UDP.
type Asker interface {
	Ask(endpoint, op, auth string, body any, out any) error
}

func Announce(op string, nodes []clientstate.Node, key *qdcrypt.Key, token string, me Device,
	wire Asker) int {
	if wire == nil || token == "" {
		return 0
	}

	var heard atomic.Int32
	var wg sync.WaitGroup
	for _, n := range nodes {
		wg.Add(1)
		go func(where string) {
			defer wg.Done()
			if err := wire.Ask(where, op, token, claim(me, token), nil); err == nil {
				heard.Add(1)
			}
		}(fmt.Sprintf("%s:%d", n.Address, n.Port))
	}
	wg.Wait()
	return int(heard.Load())
}

func AskStanding(nodes []clientstate.Node, key *qdcrypt.Key, token string, me Device,
	wire Asker) (Standing, bool) {
	if wire == nil || token == "" || len(nodes) == 0 {
		return Standing{}, false
	}

	type reply struct {
		standing Standing
		heard    bool
	}
	answers := make(chan reply, len(nodes))

	for _, n := range nodes {
		go func(where string) {
			var answer Standing
			if err := wire.Ask(where, "whoami", token, claim(me, token), &answer); err != nil {
				answers <- reply{}
				return
			}
			answers <- reply{standing: answer, heard: true}
		}(fmt.Sprintf("%s:%d", n.Address, n.Port))
	}

	var fallback Standing
	heard := false
	for range nodes {
		got := <-answers
		if !got.heard {
			continue
		}
		if got.standing.Known {
			return got.standing, true
		}
		if !heard {
			fallback, heard = got.standing, true
		}
	}
	return fallback, heard
}
