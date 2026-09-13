package hybrid

import (
	"sync/atomic"

	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
)

type Meter struct {
	Out      atomic.Uint64
	In       atomic.Uint64
	Back     atomic.Uint64
	BytesOut atomic.Uint64
	BytesIn  atomic.Uint64
}

func (m *Meter) carried(n int) {
	m.Out.Add(1)
	m.BytesOut.Add(uint64(n))
}

func (m *Meter) delivered(pkts []packet.Packet) {
	m.In.Add(uint64(len(pkts)))
	for i := range pkts {
		m.BytesIn.Add(uint64(len(pkts[i].Data)))
	}
}
