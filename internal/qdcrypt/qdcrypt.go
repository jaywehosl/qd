package qdcrypt

import (
	"crypto/rand"
	"hash/fnv"
)

const KeySize = 32

type Key [KeySize]byte

func RandomKey() (Key, error) {
	var k Key
	_, err := rand.Read(k[:])
	return k, err
}

type Exit byte

const (
	ExitDefault Exit = 0
	ExitLocal   Exit = 1
	ExitEgress  Exit = 2
)

func SessionID(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	id := h.Sum32()
	if id == 0 {
		return 1
	}
	return id
}
