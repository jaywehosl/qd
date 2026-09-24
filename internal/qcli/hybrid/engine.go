package hybrid

import (
	"context"
	"encoding/binary"
	"log"
	"net/netip"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	quic "github.com/quic-go/quic-go"

	"github.com/jaywehosl/quic-diver/internal/ippkt"
	"github.com/jaywehosl/quic-diver/internal/qcli/guard"
	"github.com/jaywehosl/quic-diver/internal/qcli/nat"
	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
	"github.com/jaywehosl/quic-diver/internal/qsrv/server/netstack"
)

const (
	maxInboundBatch = 128
	tcpSlot         = 2048
	bufSlot         = 65600
)

type Tunnel interface {
	WritePacket(b []byte) (icmp []byte, err error)
	ReadPacket(b []byte) (int, error)
}

type markedTunnel interface {
	WritePacketMarked(b []byte, mark uint64) (icmp []byte, err error)
}

type Options struct {
	Guard    *guard.Guard
	NAT      *nat.NAT
	Stack    *netstack.Stack
	Workers  int
	Meter    *Meter
	Fast     func()
	CatchDNS bool
	Direct   func(pkt []byte) bool
	Mark     func(pkt []byte) uint64
	Loud     bool
	Gateway  netip.Addr
}

type Engine struct {
	Options
	bufPool sync.Pool
	tcpPool sync.Pool

	cOutRecv, cTCP, cUDP, cBypass, cWriteErr, cOversize atomic.Uint64
	cInRecv, cInject, cInErr                            atomic.Uint64
}

func New(opts Options) *Engine {
	if opts.Workers < 1 {
		opts.Workers = 1
	}
	return &Engine{
		Options: opts,
		bufPool: sync.Pool{New: func() any { return new([bufSlot]byte) }},
		tcpPool: sync.Pool{New: func() any { return new([tcpSlot]byte) }},
	}
}

func (e *Engine) Run(ctx context.Context, src packet.Source, tun Tunnel) error {
	errc := make(chan error, e.Workers+2)

	tt := &tcpTunnel{
		ch:    make(chan []byte, 8192),
		out:   make(chan []byte, 16384),
		pool:  &e.tcpPool,
		meter: e.Meter,
	}

	ms, multi := src.(packet.MultiSource)
	var writer packet.Writer = src
	if multi {
		writer = ms.NewWriter()
	}
	go func() {
		defer e.hurry()()
		tt.injector(ctx, writer)
	}()

	defer e.Stack.Reset(tt, resetDrain)
	go func() { errc <- e.Stack.Run(ctx, tt) }()

	if multi && e.Workers > 1 {
		log.Printf("capture on %d threads, reordering possible", e.Workers)
		for i := 0; i < e.Workers; i++ {
			go func(r packet.Reader) {
				defer e.hurry()()
				e.pumpOutbound(ctx, r, src, tun, tt, errc)
			}(ms.NewReader())
		}
	} else {
		go func() {
			defer e.hurry()()
			e.pumpOutbound(ctx, src, src, tun, tt, errc)
		}()
	}
	go func() {
		defer e.hurry()()
		e.pumpInbound(ctx, src, tun, errc)
	}()
	if e.Loud {
		go e.logStats(ctx, tt, src)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errc:
		return err
	}
}

func (e *Engine) logStats(ctx context.Context, tt *tcpTunnel, src packet.Source) {
	t := time.NewTicker(statsEvery)
	defer t.Stop()
	var seen uint64
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			moved := e.cOutRecv.Load() + e.cInRecv.Load()
			if moved == seen {
				continue
			}
			seen = moved
			rcvd, dropped := quic.DatagramStats()
			var pct float64
			if rcvd+dropped > 0 {
				pct = float64(dropped) * 100 / float64(rcvd+dropped)
			}
			log.Printf("stats out: recv=%d tcp→stack=%d udp→datagram=%d bypass=%d oversize=%d | udp-in: recv=%d inject=%d",
				e.cOutRecv.Load(), e.cTCP.Load(), e.cUDP.Load(), e.cBypass.Load(),
				e.cOversize.Load(), e.cInRecv.Load(), e.cInject.Load())
			var avgBatch float64
			if b := tt.cBatches.Load(); b > 0 {
				avgBatch = float64(tt.cWrite.Load()) / float64(b)
			}
			sunk := uint64(0)
			if teller, ok := src.(interface{ Dropped() uint64 }); ok {
				sunk = teller.Dropped()
			}
			log.Printf("  bridge: push=%d drop=%d read=%d | inject=%d batches=%d (avg %.1f pkt/syscall) outDrop=%d writeErr=%d tunDrop=%d | datagram DROPPED=%.2f%%",
				tt.cPush.Load(), tt.cDrop.Load(), tt.cRead.Load(),
				tt.cWrite.Load(), tt.cBatches.Load(), avgBatch,
				tt.cOutDrop.Load(), tt.cWriteErr.Load(), sunk, pct)
			log.Printf("  stack: %s", e.Stack.DebugStats())
		}
	}
}

func (e *Engine) pumpOutbound(ctx context.Context, rd packet.Reader, src packet.Source, tun Tunnel, tt *tcpTunnel, errc chan<- error) {
	var reinject []packet.Packet
	for {
		pkts, err := rd.Recv(ctx)
		if err != nil {
			errc <- err
			return
		}
		e.cOutRecv.Add(uint64(len(pkts)))
		reinject = reinject[:0]
		for i := range pkts {
			p := &pkts[i]
			dst, ok := ippkt.Dst(p.Data)
			if !ok {
				continue
			}
			catch := e.CatchDNS && ippkt.IsDNS(p.Data)
			if !catch && (e.Guard.Bypass(dst) || e.stepsAside(p.Data)) {
				e.cBypass.Add(1)
				reinject = append(reinject, *p)
				continue
			}
			if catch || ippkt.IsTCP(p.Data) {
				e.cTCP.Add(1)
				e.Meter.carried(len(p.Data))
				tt.push(p.Data)
				continue
			}
			if expired := e.expired(p.Data); expired != nil {
				reinject = append(reinject, packet.Packet{Data: expired, Dir: packet.Inbound})
				continue
			}
			e.cUDP.Add(1)
			e.Meter.carried(len(p.Data))
			e.NAT.Outbound(p.Data)
			icmp, err := e.carry(tun, p.Data)
			if err != nil {
				e.cWriteErr.Add(1)
				continue
			}
			if len(icmp) > 0 {
				e.cOversize.Add(1)
				e.NAT.Inbound(icmp)
				reinject = append(reinject, packet.Packet{Data: icmp, Dir: packet.Inbound})
			}
		}
		if len(reinject) > 0 {
			if err := src.Send(reinject); err != nil {
				e.cInErr.Add(1)
			}
		}
	}
}

func (e *Engine) pumpInbound(ctx context.Context, src packet.Source, tun Tunnel, errc chan<- error) {
	ch := make(chan []byte, 2048)
	go func() {
		defer close(ch)
		for {
			if ctx.Err() != nil {
				return
			}
			slot := e.bufPool.Get().(*[bufSlot]byte)
			n, err := tun.ReadPacket(slot[:])
			if err != nil {
				errc <- err
				return
			}
			if n == 0 {
				e.bufPool.Put(slot)
				continue
			}
			e.cInRecv.Add(1)
			select {
			case ch <- slot[:n]:
			case <-ctx.Done():
				return
			}
		}
	}()

	batch := make([]packet.Packet, 0, maxInboundBatch)
	for {
		var first []byte
		select {
		case <-ctx.Done():
			return
		case d, ok := <-ch:
			if !ok {
				return
			}
			first = d
		}
		batch = batch[:0]
		e.prep(first, &batch)
	drain:
		for len(batch) < maxInboundBatch {
			select {
			case d, ok := <-ch:
				if !ok {
					break drain
				}
				e.prep(d, &batch)
			default:
				break drain
			}
		}
		if err := src.Send(batch); err != nil {
			e.cInErr.Add(1)
		} else {
			e.cInject.Add(uint64(len(batch)))
			e.Meter.delivered(batch)
			e.Meter.Back.Add(uint64(len(batch)))
		}
		for _, p := range batch {
			e.bufPool.Put((*[bufSlot]byte)(p.Data[:bufSlot]))
		}
	}
}

func (e *Engine) prep(data []byte, batch *[]packet.Packet) {
	e.NAT.Inbound(data)
	*batch = append(*batch, packet.Packet{Data: data, Dir: packet.Inbound})
}

type tcpTunnel struct {
	ch   chan []byte
	out  chan []byte
	pool *sync.Pool

	cPush, cDrop, cRead, cWrite, cWriteErr, cOutDrop, cBatches atomic.Uint64

	meter *Meter
}

func (t *tcpTunnel) take(pkt []byte, into chan []byte) bool {
	slot := t.pool.Get().(*[tcpSlot]byte)
	n := copy(slot[:], pkt)
	select {
	case into <- slot[:n]:
		return true
	default:
		t.pool.Put(slot)
		return false
	}
}

func (t *tcpTunnel) give(b []byte) { t.pool.Put((*[tcpSlot]byte)(b[:tcpSlot])) }

func (t *tcpTunnel) push(pkt []byte) {
	if t.take(pkt, t.ch) {
		t.cPush.Add(1)
	} else {
		t.cDrop.Add(1)
	}
}

func (t *tcpTunnel) ReadPacket(b []byte) (int, error) {
	data, ok := <-t.ch
	if !ok {
		return 0, context.Canceled
	}
	n := copy(b, data)
	t.give(data)
	t.cRead.Add(1)
	return n, nil
}

func (t *tcpTunnel) WritePacket(b []byte) ([]byte, error) {
	if !t.take(b, t.out) {
		t.cOutDrop.Add(1)
	}
	return nil, nil
}

const injectGather = 300 * time.Microsecond

func (t *tcpTunnel) injector(ctx context.Context, w packet.Writer) {
	batch := make([]packet.Packet, 0, maxInboundBatch)
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	if !timer.Stop() {
		<-timer.C
	}

	for {
		var first []byte
		select {
		case <-ctx.Done():
			return
		case d, ok := <-t.out:
			if !ok {
				return
			}
			first = d
		}
		batch = append(batch[:0], packet.Packet{Data: first, Dir: packet.Inbound})

		timer.Reset(injectGather)
	drain:
		for len(batch) < maxInboundBatch {
			select {
			case d, ok := <-t.out:
				if !ok {
					break drain
				}
				batch = append(batch, packet.Packet{Data: d, Dir: packet.Inbound})
			case <-timer.C:
				break drain
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		if err := w.Send(batch); err != nil {
			t.cWriteErr.Add(1)
		} else {
			t.cWrite.Add(uint64(len(batch)))
			t.meter.delivered(batch)
		}
		t.cBatches.Add(1)
		for _, p := range batch {
			t.give(p.Data)
		}
	}
}

func (e *Engine) stepsAside(pkt []byte) bool {
	return e.Direct != nil && e.Direct(pkt)
}

func (e *Engine) carry(tun Tunnel, pkt []byte) ([]byte, error) {
	marked, ok := tun.(markedTunnel)
	if !ok || e.Mark == nil {
		return tun.WritePacket(pkt)
	}
	return marked.WritePacketMarked(pkt, e.Mark(pkt))
}

func (e *Engine) hurry() func() {
	if e.Fast == nil {
		return func() {}
	}
	runtime.LockOSThread()
	e.Fast()
	return runtime.UnlockOSThread
}

const resetDrain = 300 * time.Millisecond

const statsEvery = time.Minute

func (e *Engine) expired(pkt []byte) []byte {
	if !e.Gateway.Is4() || len(pkt) < 28 || pkt[0]>>4 != 4 || pkt[8] > 1 {
		return nil
	}
	ihl := int(pkt[0]&0x0F) * 4
	if ihl < 20 || len(pkt) < ihl+8 {
		return nil
	}
	msg := append([]byte{11, 0, 0, 0, 0, 0, 0, 0}, pkt[:ihl+8]...)
	binary.BigEndian.PutUint16(msg[2:], ippkt.Checksum(msg))

	out := make([]byte, 20+len(msg))
	out[0], out[8], out[9] = 0x45, 64, 1
	binary.BigEndian.PutUint16(out[2:], uint16(len(out)))
	gw := e.Gateway.As4()
	copy(out[12:16], gw[:])
	copy(out[16:20], pkt[12:16])
	binary.BigEndian.PutUint16(out[10:], ippkt.Checksum(out[:20]))
	copy(out[20:], msg)
	return out
}
