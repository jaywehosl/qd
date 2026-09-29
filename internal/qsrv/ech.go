package qsrv

import (
	"crypto/ecdh"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"errors"
)

func ECHKey(priv []byte, publicName string) (tls.EncryptedClientHelloKey, []byte, error) {
	if publicName == "" || len(publicName) > 255 {
		return tls.EncryptedClientHelloKey{}, nil, errors.New("ech: the public name must be 1 to 255 bytes")
	}
	key, err := ecdh.X25519().NewPrivateKey(priv)
	if err != nil {
		return tls.EncryptedClientHelloKey{}, nil, err
	}
	pub := key.PublicKey().Bytes()
	id := sha256.Sum256(pub)

	body := []byte{id[0]}
	body = binary.BigEndian.AppendUint16(body, 0x0020)
	body = binary.BigEndian.AppendUint16(body, uint16(len(pub)))
	body = append(body, pub...)
	suites := []uint16{0x0001, 0x0001, 0x0001, 0x0003}
	body = binary.BigEndian.AppendUint16(body, uint16(2*len(suites)))
	for _, v := range suites {
		body = binary.BigEndian.AppendUint16(body, v)
	}
	body = append(body, 0, byte(len(publicName)))
	body = append(body, publicName...)
	body = binary.BigEndian.AppendUint16(body, 0)

	config := binary.BigEndian.AppendUint16(nil, 0xfe0d)
	config = binary.BigEndian.AppendUint16(config, uint16(len(body)))
	config = append(config, body...)
	list := binary.BigEndian.AppendUint16(nil, uint16(len(config)))
	list = append(list, config...)
	return tls.EncryptedClientHelloKey{Config: config, PrivateKey: priv, SendAsRetry: true}, list, nil
}
