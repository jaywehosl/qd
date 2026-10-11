package quicconn

import (
	"encoding/binary"
	"hash/fnv"

	"github.com/quic-go/quic-go"
)

func init() { quic.DatagramFlow = flowOf }

func flowOf(data []byte) uint32 {
	if len(data) == 0 {
		return 0
	}
	head := 1 << (data[0] >> 6)
	if len(data) < head {
		return 0
	}
	h := fnv.New32a()
	h.Write(data[:head])
	if rest := data[head:]; len(rest) > 0 {
		if mark := 1 << (rest[0] >> 6); len(rest) > mark {
			tuple(h, rest[mark:])
		}
	}
	return h.Sum32()
}

type hasher interface{ Write([]byte) (int, error) }

func tuple(h hasher, pkt []byte) {
	if len(pkt) < 20 {
		return
	}
	switch pkt[0] >> 4 {
	case 4:
		ihl := int(pkt[0]&0x0f) * 4
		if ihl < 20 || len(pkt) < ihl || int(binary.BigEndian.Uint16(pkt[2:4])) != len(pkt) {
			return
		}
		h.Write(pkt[9:10])
		h.Write(pkt[12:20])
		if binary.BigEndian.Uint16(pkt[6:8])&0x1fff == 0 && len(pkt) >= ihl+4 {
			h.Write(pkt[ihl : ihl+4])
		}
	case 6:
		if len(pkt) < 40 || int(binary.BigEndian.Uint16(pkt[4:6]))+40 != len(pkt) {
			return
		}
		h.Write(pkt[6:7])
		h.Write(pkt[8:40])
		if len(pkt) >= 44 {
			h.Write(pkt[40:44])
		}
	}
}
