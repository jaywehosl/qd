//go:build windows

package windivert

import (
	"context"
	"encoding/binary"
	"errors"
	"sync/atomic"

	"golang.org/x/sys/windows"

	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
)

const (
	recvBufBytes  = 2 * 1024 * 1024
	queueLenBoost = 8192
)

var (
	errShortIP   = errors.New("windivert: IP-пакет короче заголовка")
	errIPVersion = errors.New("windivert: неизвестная версия IP")
)

type Source struct {
	h   windows.Handle
	mtu int

	recvBuf  []byte
	recvAddr []Address
	out      []packet.Packet

	sendBuf  []byte
	sendAddr []Address

	lastIfIdx atomic.Uint32
}

func (s *Source) NewReader() packet.Reader {
	return &reader{
		s:     s,
		buf:   make([]byte, recvBufBytes),
		addrs: make([]Address, BatchMax),
		out:   make([]packet.Packet, 0, BatchMax),
	}
}

func (s *Source) NewWriter() packet.Writer {
	return &writer{
		s:     s,
		buf:   make([]byte, 0, recvBufBytes),
		addrs: make([]Address, 0, BatchMax),
	}
}

type reader struct {
	s     *Source
	buf   []byte
	addrs []Address
	out   []packet.Packet
}

func (r *reader) Recv(ctx context.Context) ([]packet.Packet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	packetLen, addrCount, err := recvEx(r.s.h, r.buf, r.addrs)
	if err != nil {
		return nil, err
	}
	out, err := splitBatch(r.buf[:packetLen], r.addrs[:addrCount], r.out[:0])
	if err != nil {
		return out, err
	}
	if n := len(out); n > 0 && out[n-1].IfIndex != 0 {
		r.s.lastIfIdx.Store(out[n-1].IfIndex)
	}
	r.out = out
	return out, nil
}

type writer struct {
	s     *Source
	buf   []byte
	addrs []Address
}

func (w *writer) Send(pkts []packet.Packet) error {
	for len(pkts) > 0 {
		n := len(pkts)
		if n > BatchMax {
			n = BatchMax
		}
		if err := w.sendChunk(pkts[:n]); err != nil {
			return err
		}
		pkts = pkts[n:]
	}
	return nil
}

func (w *writer) sendChunk(pkts []packet.Packet) error {
	w.buf = w.buf[:0]
	w.addrs = w.addrs[:0]
	for i := range pkts {
		p := &pkts[i]
		if len(p.Data) == 0 {
			continue
		}
		w.buf = append(w.buf, p.Data...)
		var a Address
		a.SetLayer(LayerNetwork)
		a.SetOutbound(p.Dir == packet.Outbound)
		idx := p.IfIndex
		if idx == 0 {
			idx = w.s.lastIfIdx.Load()
		}
		a.SetIfIdx(idx)
		w.addrs = append(w.addrs, a)
	}
	if len(w.addrs) == 0 {
		return nil
	}
	_, err := sendEx(w.s.h, w.buf, w.addrs)
	return err
}

var (
	_ packet.MultiSource = (*Source)(nil)
	_ packet.Reader      = (*reader)(nil)
	_ packet.Writer      = (*writer)(nil)
)

func Open(dllPath, filter string, flags uint64) (*Source, error) {
	if err := Load(dllPath); err != nil {
		return nil, err
	}
	h, err := open(filter, LayerNetwork, 0, flags)
	if err != nil {
		return nil, err
	}
	_ = setParam(h, ParamQueueLength, queueLenBoost)

	return &Source{
		h:        h,
		mtu:      1500,
		recvBuf:  make([]byte, recvBufBytes),
		recvAddr: make([]Address, BatchMax),
		out:      make([]packet.Packet, 0, BatchMax),
		sendBuf:  make([]byte, 0, recvBufBytes),
		sendAddr: make([]Address, 0, BatchMax),
	}, nil
}

func (s *Source) Recv(ctx context.Context) ([]packet.Packet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	packetLen, addrCount, err := recvEx(s.h, s.recvBuf, s.recvAddr)
	if err != nil {
		return nil, err
	}
	out, err := splitBatch(s.recvBuf[:packetLen], s.recvAddr[:addrCount], s.out[:0])
	if err != nil {
		return out, err
	}
	if n := len(out); n > 0 && out[n-1].IfIndex != 0 {
		s.lastIfIdx.Store(out[n-1].IfIndex)
	}
	return out, nil
}

func (s *Source) Send(pkts []packet.Packet) error {
	for len(pkts) > 0 {
		n := len(pkts)
		if n > BatchMax {
			n = BatchMax
		}
		if err := s.sendChunk(pkts[:n]); err != nil {
			return err
		}
		pkts = pkts[n:]
	}
	return nil
}

func (s *Source) sendChunk(pkts []packet.Packet) error {
	if len(pkts) == 0 {
		return nil
	}
	s.sendBuf = s.sendBuf[:0]
	s.sendAddr = s.sendAddr[:0]
	for i := range pkts {
		p := &pkts[i]
		if len(p.Data) == 0 {
			continue
		}
		s.sendBuf = append(s.sendBuf, p.Data...)
		var a Address
		a.SetLayer(LayerNetwork)
		a.SetOutbound(p.Dir == packet.Outbound)
		idx := p.IfIndex
		if idx == 0 {
			idx = s.lastIfIdx.Load()
		}
		a.SetIfIdx(idx)
		s.sendAddr = append(s.sendAddr, a)
	}
	_, err := sendEx(s.h, s.sendBuf, s.sendAddr)
	return err
}

func (s *Source) MTU() int { return s.mtu }

func (s *Source) Close() error {
	_ = shutdown(s.h, ShutdownBoth)
	return closeHandle(s.h)
}

func splitBatch(buf []byte, addrs []Address, out []packet.Packet) ([]packet.Packet, error) {
	off := 0
	for i := range addrs {
		if off >= len(buf) {
			break
		}
		n, err := ipPacketLen(buf[off:])
		if err != nil {
			return out, err
		}
		if n == 0 || off+n > len(buf) {
			break
		}
		a := &addrs[i]
		pkt := buf[off : off+n]
		if needsChecksum(a, pkt) {
			calcChecksums(pkt)
		}
		dir := packet.Inbound
		if a.Outbound() {
			dir = packet.Outbound
		}
		out = append(out, packet.Packet{
			Data:    pkt,
			Dir:     dir,
			IfIndex: a.IfIdx(),
		})
		off += n
	}
	return out, nil
}

func needsChecksum(a *Address, pkt []byte) bool {
	if len(pkt) < 1 {
		return false
	}
	v6 := pkt[0]>>4 == 6
	if !v6 && !a.IPChecksumValid() {
		return true
	}
	var proto byte
	if v6 {
		if len(pkt) < 40 {
			return false
		}
		proto = pkt[6]
	} else {
		if len(pkt) < 20 {
			return false
		}
		proto = pkt[9]
	}
	switch proto {
	case 6:
		return !a.TCPChecksumValid()
	case 17:
		return !a.UDPChecksumValid()
	}
	return false
}

func ipPacketLen(b []byte) (int, error) {
	if len(b) < 1 {
		return 0, errShortIP
	}
	switch b[0] >> 4 {
	case 4:
		if len(b) < 20 {
			return 0, errShortIP
		}
		return int(binary.BigEndian.Uint16(b[2:4])), nil
	case 6:
		if len(b) < 40 {
			return 0, errShortIP
		}
		return 40 + int(binary.BigEndian.Uint16(b[4:6])), nil
	default:
		return 0, errIPVersion
	}
}

var _ packet.Source = (*Source)(nil)
