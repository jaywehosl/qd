package qdmobile

import (
	"context"

	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
)

type onlyV4 struct {
	packet.Source
}

func watched(src packet.Source) packet.Source { return onlyV4{Source: src} }

func (o onlyV4) Recv(ctx context.Context) ([]packet.Packet, error) {
	pkts, err := o.Source.Recv(ctx)

	kept := pkts[:0]
	for i := range pkts {
		if len(pkts[i].Data) > 0 && pkts[i].Data[0]>>4 == 6 {
			continue
		}
		kept = append(kept, pkts[i])
	}
	return kept, err
}
