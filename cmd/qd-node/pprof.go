//go:build linux

package main

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
)

func servePprof() {
	at := os.Getenv("QD_PPROF")
	if at == "" {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	fmt.Printf("pprof      listening on %s\n", at)
	go http.ListenAndServe(at, mux)
}
