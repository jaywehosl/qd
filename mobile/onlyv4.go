package qdmobile

import (
	"context"

	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
)

// onlyV4 роняет IPv6 на входе в туннель. Забрать его приходится — иначе система
// пустит трафик мимо, — но узел может не иметь IPv6 вовсе, и тогда каждый такой
// флоу висит у него на дозвоне и держит поток, а обычные запросы ждут за ним до
// таймаута. Уроненный пакет дешевле: приложение сразу идёт по IPv4.
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
