//go:build windows || linux

package main

import (
	"fmt"
	"net/http"

	"github.com/jaywehosl/qd/internal/localapi"
)

func startLocalUI(host string, port int, client http.Handler, admin *adminUI, isAdmin func() bool) (*localapi.Server, error) {
	cfg := localapi.Config{Client: client, IsAdmin: isAdmin}
	if admin != nil {
		cfg.Admin = admin
	}
	if !embedded {
		if err := withPage(&cfg); err != nil {
			return nil, err
		}
	}

	srv, err := localapi.New(cfg)
	if err != nil {
		return nil, err
	}

	l, err := srv.ListenOn(host, port)
	if err != nil {
		return nil, err
	}

	go func() {
		if err := http.Serve(l, srv); err != nil {
			fmt.Printf("local ui: %v\n", err)
		}
	}()

	return srv, nil
}
