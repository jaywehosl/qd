//go:build windows && core

package main

import (
	"github.com/jaywehosl/qd/internal/clientapi"
	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/localapi"
)

const headless = true

func startShell(db *clientstate.DB, tun *tunnel, ui *localapi.Server, api *clientapi.API, quit chan struct{}, stop <-chan struct{}) (bool, func()) {
	return false, func() {}
}

func openPage(url string) {}

func bindPane(statePath, url, token string) {}
