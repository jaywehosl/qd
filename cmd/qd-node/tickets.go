//go:build linux

package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"os"
	"path/filepath"
	"time"
)

const ticketWeek = int64(7 * 24 * time.Hour / time.Second)

func holdTickets(conf *tls.Config, dir string) error {
	secret, err := ticketSecret(filepath.Join(dir, "ticket.secret"))
	if err != nil {
		return err
	}

	conf.WrapSession = func(_ tls.ConnectionState, s *tls.SessionState) ([]byte, error) {
		plain, err := s.Bytes()
		if err != nil {
			return nil, err
		}
		week := uint64(time.Now().Unix() / ticketWeek)
		aead, err := ticketAEAD(secret, week)
		if err != nil {
			return nil, err
		}
		head := 8 + aead.NonceSize()
		out := make([]byte, head, head+len(plain)+aead.Overhead())
		binary.BigEndian.PutUint64(out, week)
		if _, err := rand.Read(out[8:head]); err != nil {
			return nil, err
		}
		return aead.Seal(out, out[8:head], plain, out[:8]), nil
	}

	conf.UnwrapSession = func(identity []byte, _ tls.ConnectionState) (*tls.SessionState, error) {
		if len(identity) < 8 {
			return nil, nil
		}
		week := binary.BigEndian.Uint64(identity)
		now := uint64(time.Now().Unix() / ticketWeek)
		if week != now && week+1 != now {
			return nil, nil
		}
		aead, err := ticketAEAD(secret, week)
		if err != nil {
			return nil, nil
		}
		head := 8 + aead.NonceSize()
		if len(identity) < head {
			return nil, nil
		}
		plain, err := aead.Open(nil, identity[8:head], identity[head:], identity[:8])
		if err != nil {
			return nil, nil
		}
		s, err := tls.ParseSessionState(plain)
		if err != nil {
			return nil, nil
		}
		return s, nil
	}
	return nil
}

func ticketAEAD(secret []byte, week uint64) (cipher.AEAD, error) {
	mac := hmac.New(sha256.New, secret)
	var label [8]byte
	binary.BigEndian.PutUint64(label[:], week)
	mac.Write([]byte("qd session ticket"))
	mac.Write(label[:])
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func ticketSecret(path string) ([]byte, error) {
	if held, err := os.ReadFile(path); err == nil && len(held) == 32 {
		return held, nil
	}
	fresh := make([]byte, 32)
	if _, err := rand.Read(fresh); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, fresh, 0o600); err != nil {
		return nil, err
	}
	return fresh, nil
}
