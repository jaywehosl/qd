package qdcrypt

import "hash/fnv"

const KeySize = 32

type Key [KeySize]byte

type Exit byte

const (
	ExitLocal  Exit = 1
	ExitEgress Exit = 2
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
