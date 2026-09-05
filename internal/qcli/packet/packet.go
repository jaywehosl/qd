package packet

import "context"

type Direction uint8

const (
	Outbound Direction = iota
	Inbound
)

type Packet struct {
	Data    []byte
	Dir     Direction
	PID     uint32
	IfIndex uint32
}

type Source interface {
	Recv(ctx context.Context) ([]Packet, error)

	Send(pkts []Packet) error

	MTU() int

	Close() error
}

type MultiSource interface {
	Source
	NewReader() Reader
	NewWriter() Writer
}

type Reader interface {
	Recv(ctx context.Context) ([]Packet, error)
}

type Writer interface {
	Send(pkts []Packet) error
}
