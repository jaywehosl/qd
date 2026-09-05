package uplink

import (
	"context"
	"io"
)

type Conn interface {
	SendDatagram(b []byte) error

	RecvDatagram(ctx context.Context) ([]byte, error)

	OpenStream(ctx context.Context) (Stream, error)

	MaxDatagramSize() int

	Close() error
}

type Stream interface {
	io.ReadWriteCloser
}

type Dialer interface {
	Dial(ctx context.Context, endpoint string) (Conn, error)
}
