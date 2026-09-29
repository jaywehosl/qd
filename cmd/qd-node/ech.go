//go:build linux

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"path/filepath"

	"github.com/jaywehosl/qd/internal/qsrv"
)

func holdECH(conf *tls.Config, dir, publicName string) ([]byte, error) {
	secret, err := ticketSecret(filepath.Join(dir, "ticket.secret"))
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("qd ech"))
	key, list, err := qsrv.ECHKey(mac.Sum(nil), publicName)
	if err != nil {
		return nil, err
	}
	conf.EncryptedClientHelloKeys = []tls.EncryptedClientHelloKey{key}
	return list, nil
}
