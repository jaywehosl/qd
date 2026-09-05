//go:build windows

package main

import (
	"fmt"
	"net/http"

	"github.com/jaywehosl/quic-diver/internal/localapi"
	"github.com/jaywehosl/quic-diver/web"
)

func startLocalUI(host string, port int, client, admin http.Handler, isAdmin func() bool) (*localapi.Server, error) {
	page, err := web.Handler("")
	if err != nil {
		return nil, err
	}

	srv, err := localapi.New(localapi.Config{
		Page:    page,
		Client:  client,
		Admin:   admin,
		IsAdmin: isAdmin,
		Index: func(token string) ([]byte, error) {
			return web.IndexWith(map[string]string{
				"X_UI_BASE_PATH": "/",
				"QD_TOKEN":       token,
			})
		},
	})
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
