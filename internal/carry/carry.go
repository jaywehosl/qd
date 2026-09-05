package carry

import (
	"encoding/binary"
	"errors"
	"net"
	"runtime"
	"sync/atomic"
	"time"
)

type Device interface {
	Receive() ([]byte, error)
	Release(pkt []byte)
	Send(pkt []byte) error
}

type Stats struct {
	Out      atomic.Uint64
	In       atomic.Uint64
	BytesOut atomic.Uint64
	BytesIn  atomic.Uint64
	TunErr   atomic.Uint64
	SendErr  atomic.Uint64
	SendDrop atomic.Uint64
	Retries  atomic.Uint64
	Revoked  atomic.Uint64
	V6Drop   atomic.Uint64
	Reorder  atomic.Uint64
	Gaps     atomic.Uint64

	lastSeq atomic.Uint64
}

func Write(conn *net.UDPConn, frame []byte, st *Stats) bool {
	for attempt := 0; ; attempt++ {
		_, err := conn.Write(frame)
		if err == nil {
			if attempt > 0 {
				st.Retries.Add(uint64(attempt))
			}
			return true
		}

		if !errors.Is(err, noBuffers) {
			st.SendErr.Add(1)
			return false
		}
		if attempt >= 64 {
			st.SendDrop.Add(1)
			return false
		}

		if attempt < 8 {
			runtime.Gosched()
			continue
		}
		time.Sleep(50 * time.Microsecond)
	}
}

func (st *Stats) Track(seq uint32) {
	for {
		prev := st.lastSeq.Load()
		if prev == 0 {
			if st.lastSeq.CompareAndSwap(prev, uint64(seq)) {
				return
			}
			continue
		}

		last := uint32(prev)
		diff := int32(seq - last)

		if diff <= 0 {
			st.Reorder.Add(1)
			return
		}
		if !st.lastSeq.CompareAndSwap(prev, uint64(seq)) {
			continue
		}
		if diff > 1 {
			st.Gaps.Add(uint64(diff - 1))
		}
		return
	}
}

func FlowHash(pkt []byte) uint32 {
	if len(pkt) < 20 || pkt[0]>>4 != 4 {
		return 0
	}
	ihl := int(pkt[0]&0x0F) * 4
	h := binary.BigEndian.Uint32(pkt[12:16])
	h = h*2654435761 + binary.BigEndian.Uint32(pkt[16:20])

	if (pkt[9] == 6 || pkt[9] == 17) && len(pkt) >= ihl+4 {
		h = h*2654435761 + uint32(binary.BigEndian.Uint16(pkt[ihl:ihl+2]))
		h = h*2654435761 + uint32(binary.BigEndian.Uint16(pkt[ihl+2:ihl+4]))
	}
	h ^= h >> 16
	return h
}
