//go:build (windows || linux) && !core

package main

import (
	"github.com/jaywehosl/qd/internal/localapi"
	"github.com/jaywehosl/qd/web"
)

func withPage(cfg *localapi.Config) error {
	page, err := web.Handler("")
	if err != nil {
		return err
	}
	cfg.Page = page
	cfg.Guarded = guardPage
	cfg.Index = func(token string) ([]byte, error) {
		return web.IndexWith(map[string]string{
			"QD_TOKEN": token,
		})
	}
	return nil
}
