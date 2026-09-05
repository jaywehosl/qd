package clientstate

import "github.com/jaywehosl/quic-diver/internal/qdcrypt"

func SessionID(key string) uint32 { return qdcrypt.SessionID(key) }
