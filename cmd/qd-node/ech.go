//go:build linux

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/jaywehosl/qd/internal/qsrv"
	"github.com/jaywehosl/qd/internal/store"
)

const echRecheck = 5 * time.Second

type echHolder struct {
	db      func() *store.DB
	network string
	renamed func(name string)

	mu      sync.Mutex
	checked time.Time
	name    string
	secret  string
	keys    []tls.EncryptedClientHelloKey
	list    []byte
}

func newECH(db func() *store.DB, network string, settings store.NetworkSettings, renamed func(string)) *echHolder {
	h := &echHolder{db: db, network: network, renamed: renamed, checked: time.Now()}
	h.apply(settings.ECHName, settings.ECHSecret)
	go func() {
		for range time.Tick(echRecheck) {
			h.current()
		}
	}()
	return h
}

func (h *echHolder) current() ([]tls.EncryptedClientHelloKey, []byte) {
	if h == nil {
		return []tls.EncryptedClientHelloKey{}, nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if time.Since(h.checked) > echRecheck {
		h.checked = time.Now()
		if settings, err := h.db().NetworkSettings(); err == nil && (settings.ECHName != h.name || settings.ECHSecret != h.secret) {
			h.apply(settings.ECHName, settings.ECHSecret)
		}
	}
	return h.keys, h.list
}

func (h *echHolder) apply(name, secret string) {
	renamed := name != h.name
	h.name, h.secret = name, secret
	h.keys, h.list = []tls.EncryptedClientHelloKey{}, nil
	if name != "" {
		seed := secret
		if seed == "" {
			seed = h.network
		}
		mac := hmac.New(sha256.New, []byte(seed))
		mac.Write([]byte("qd ech " + name))
		key, list, err := qsrv.ECHKey(mac.Sum(nil), name)
		if err != nil {
			fmt.Printf("ech        off: %v\n", err)
		} else {
			h.keys, h.list = []tls.EncryptedClientHelloKey{key}, list
			fmt.Printf("ech        on, the outer hello names %s\n", name)
		}
	} else {
		fmt.Printf("ech        off\n")
	}
	if renamed && h.renamed != nil {
		go h.renamed(name)
	}
}

func (h *echHolder) serve(conf *tls.Config) {
	conf.GetEncryptedClientHelloKeys = func(*tls.ClientHelloInfo) ([]tls.EncryptedClientHelloKey, error) {
		keys, _ := h.current()
		return keys, nil
	}
}

func (h *echHolder) publicList(string) []byte {
	_, list := h.current()
	return list
}
