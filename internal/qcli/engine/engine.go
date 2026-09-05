package engine

import (
	"context"

	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
)

type PacketTunnel interface {
	WritePacket(b []byte) (icmp []byte, err error)
	ReadPacket(b []byte) (int, error)
}

type Rewriter interface {
	Outbound(pkt []byte)
	Inbound(pkt []byte)
}

type Engine interface {
	Run(ctx context.Context, src packet.Source, tun PacketTunnel) error
}

type MarkedTunnel interface {
	WritePacketMarked(b []byte, mark uint64) (icmp []byte, err error)
}
