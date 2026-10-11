//go:build linux

package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jaywehosl/qd/internal/qdcrypt"
	"github.com/jaywehosl/qd/internal/qsrv"
)

const abroadWait = 2 * time.Second

func (state *controlState) resolveAbroad(auth, device string, exit bool, query []byte) ([]byte, bool) {
	if auth == "" || !exit {
		return nil, false
	}
	session := qdcrypt.SessionID(auth)
	if !state.gate.exits(session) {
		return nil, false
	}

	var seat uint32
	if device != "" {
		seat = qsrv.SeatFor(session, device)
	} else {
		session = 0
	}
	return state.askAbroad(query, seat, session)
}

func (state *controlState) askAbroad(query []byte, seat, session uint32) ([]byte, bool) {
	body, err := json.Marshal(map[string]any{"query": query})
	if err != nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), abroadWait)
	defer cancel()
	raw, err := state.node.AskExit(ctx, seat, session, "dns", body)
	if err != nil {
		return nil, false
	}

	var got struct {
		Answer []byte `json:"answer"`
	}
	if json.Unmarshal(raw, &got) != nil || len(got.Answer) < 12 {
		return nil, false
	}
	return got.Answer, true
}
