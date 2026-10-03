//go:build (windows || linux) && core

package main

import (
	"net/http"

	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/localapi"
	"github.com/jaywehosl/qd/internal/qdcrypt"
)

func withPage(cfg *localapi.Config) error { return nil }

type adminUI struct{}

func newAdminUI(*qdcrypt.Key, *clientstate.DB) *adminUI { return nil }

func (*adminUI) SetKey(*qdcrypt.Key) {}

func (*adminUI) Peers() []string { return nil }

func (*adminUI) Feed() []localapi.Push { return nil }

func (*adminUI) ServeHTTP(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }

const buildKind = "core"
