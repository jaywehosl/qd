// Package qdcrypt держит сетевой ключ и то, что из него считается.
//
// Своего шифрования здесь больше нет: кадры на chacha20 нужны были, пока
// управление и трафик ехали UDP-датаграммами. Поверх QUIC всё уже зашифровано
// TLS, а границы сообщений даёт сам транспорт.
package qdcrypt

import (
	"crypto/rand"
	"hash/fnv"
)

const KeySize = 32

// Key — сетевой ключ: им клиент открывает дверь управления на узле.
type Key [KeySize]byte

func RandomKey() (Key, error) {
	var k Key
	_, err := rand.Read(k[:])
	return k, err
}

// Exit — куда клиент просит выпустить трафик.
type Exit byte

const (
	ExitDefault Exit = 0
	ExitLocal   Exit = 1
	ExitEgress  Exit = 2
)

// SessionID — номер сессии клиента, посчитанный из ключа подписки. Считается
// так же, как считался на XDP, поэтому панель и присутствие видят те же числа.
func SessionID(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	id := h.Sum32()
	if id == 0 {
		return 1
	}
	return id
}
