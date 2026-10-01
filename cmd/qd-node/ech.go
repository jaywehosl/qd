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
	secret  string
	renamed func(name string)

	mu      sync.Mutex
	checked time.Time
	name    string
	keys    []tls.EncryptedClientHelloKey
	list    []byte
}

func newECH(db func() *store.DB, secret, name string, renamed func(string)) *echHolder {
	h := &echHolder{db: db, secret: secret, renamed: renamed, checked: time.Now()}
	h.apply(name)
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
		if settings, err := h.db().NetworkSettings(); err == nil && settings.ECHName != h.name {
			h.apply(settings.ECHName)
		}
	}
	return h.keys, h.list
}

func (h *echHolder) apply(name string) {
	h.name = name
	h.keys, h.list = []tls.EncryptedClientHelloKey{}, nil
	if name != "" {
		mac := hmac.New(sha256.New, []byte(h.secret))
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
	if h.renamed != nil {
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
