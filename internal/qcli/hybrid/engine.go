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

	"github.com/jaywehosl/quic-diver/internal/qcli/engine"
	"github.com/jaywehosl/quic-diver/internal/qcli/guard"
	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
	"github.com/jaywehosl/quic-diver/internal/qsrv/server/netstack"
)

const (
	maxInboundBatch = 128
	tcpSlot         = 2048
)

type Engine struct {
	guard       *guard.Guard
	rewriter    engine.Rewriter
	ns          atomic.Pointer[netstack.Stack]
	recvWorkers int
	bufPool     sync.Pool
	tcpPool     sync.Pool

	cOutRecv, cTCP, cUDP, cBypass, cWriteErr, cOversize atomic.Uint64
	loud                                                bool
	cInRecv, cInject, cInErr                            atomic.Uint64

	meter    *Meter
	fast     atomic.Pointer[func()]
	catchDNS atomic.Bool
	direct   atomic.Pointer[func([]byte) bool]
	mark     atomic.Pointer[func([]byte) uint64]
}

func New(g *guard.Guard, rw engine.Rewriter, ns *netstack.Stack, recvWorkers int, meter *Meter) *Engine {
	if recvWorkers < 1 {
		recvWorkers = 1
	}
	e := &Engine{
		guard:       g,
		rewriter:    rw,
		recvWorkers: recvWorkers,
		meter:       meter,
		bufPool:     sync.Pool{New: func() any { return make([]byte, 65600) }},
		tcpPool:     sync.Pool{New: func() any { return make([]byte, tcpSlot) }},
	}
	e.ns.Store(ns)
	return e
}

func (e *Engine) Run(ctx context.Context, src packet.Source, tun engine.PacketTunnel) error {
	errc := make(chan error, 3)

	tt := &tcpTunnel{
		src:   src,
		ch:    make(chan []byte, 8192),
		out:   make(chan []byte, 16384),
		pool:  &e.tcpPool,
		meter: e.meter,
	}
	if ms, ok := src.(packet.MultiSource); ok {
		go func(w packet.Writer) {
			defer e.hurry()()
			tt.injector(ctx, w)
		}(ms.NewWriter())
	} else {
		go func() {
			defer e.hurry()()
			tt.injector(ctx, src)
		}()
	}

	go func() { errc <- e.ns.Load().Run(ctx, tt) }()

	if ms, ok := src.(packet.MultiSource); ok && e.recvWorkers > 1 {
		log.Printf("захват в %d потоков (внимание: возможен reordering)", e.recvWorkers)
		for i := 0; i < e.recvWorkers; i++ {
			go func(r packet.Reader) {
				defer e.hurry()()
				e.pumpOutboundReader(ctx, r, src, tun, tt, errc)
			}(ms.NewReader())
		}
	} else {
		go func() {
			defer e.hurry()()
			e.pumpOutbound(ctx, src, tun, tt, errc)
		}()
	}
	go func() {
		defer e.hurry()()
		e.pumpInbound(ctx, src, tun, errc)
	}()
	if e.loud {
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
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rcvd, dropped := quic.DatagramStats()
			var pct float64
			if rcvd+dropped > 0 {
				pct = float64(dropped) * 100 / float64(rcvd+dropped)
			}
			log.Printf("stats out: recv=%d tcp→стек=%d udp→датаграмма=%d bypass=%d oversize=%d | udp-in: recv=%d inject=%d",
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
			log.Printf("  мост: push=%d drop=%d read=%d | inject=%d батчей=%d (avg %.1f пак/syscall) outDrop=%d writeErr=%d tunDrop=%d | datagram DROPPED=%.2f%%",
				tt.cPush.Load(), tt.cDrop.Load(), tt.cRead.Load(),
				tt.cWrite.Load(), tt.cBatches.Load(), avgBatch,
				tt.cOutDrop.Load(), tt.cWriteErr.Load(), sunk, pct)
			log.Printf("  стек: %s", e.ns.Load().DebugStats())
		}
	}
}

func (e *Engine) pumpOutbound(ctx context.Context, src packet.Source, tun engine.PacketTunnel, tt *tcpTunnel, errc chan<- error) {
	e.pumpOutboundReader(ctx, srcReader{src}, src, tun, tt, errc)
}

type srcReader struct{ s packet.Source }

func (r srcReader) Recv(ctx context.Context) ([]packet.Packet, error) { return r.s.Recv(ctx) }

func (e *Engine) pumpOutboundReader(ctx context.Context, rd packet.Reader, src packet.Source, tun engine.PacketTunnel, tt *tcpTunnel, errc chan<- error) {
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
			dst, ok := dstAddr(p.Data)
			if !ok {
				continue
			}
			catch := e.catchDNS.Load() && isDNS(p.Data)
			if !catch {
				if (e.guard != nil && e.guard.Bypass(dst)) || e.stepsAside(p.Data) {
					e.cBypass.Add(1)
					reinject = append(reinject, *p)
					continue
				}
			}
			if isTCP(p.Data) || catch {
				e.cTCP.Add(1)
				e.meter.carried(len(p.Data))
				tt.push(p.Data)
				continue
			}
			e.cUDP.Add(1)
			e.meter.carried(len(p.Data))
			if e.rewriter != nil {
				e.rewriter.Outbound(p.Data)
			}
			icmp, err := e.carry(tun, p.Data)
			if err != nil {
				e.cWriteErr.Add(1)
				continue
			}
			if len(icmp) > 0 {
				e.cOversize.Add(1)
				if e.rewriter != nil {
					e.rewriter.Inbound(icmp)
				}
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

func (e *Engine) pumpInbound(ctx context.Context, src packet.Source, tun engine.PacketTunnel, errc chan<- error) {
	ch := make(chan []byte, 2048)
	go func() {
		defer close(ch)
		for {
			if ctx.Err() != nil {
				return
			}
			buf := e.bufPool.Get().([]byte)
			n, err := tun.ReadPacket(buf)
			if err != nil {
				errc <- err
				return
			}
			if n == 0 {
				e.bufPool.Put(buf)
				continue
			}
			e.cInRecv.Add(1)
			select {
			case ch <- buf[:n]:
			case <-ctx.Done():
				return
			}
		}
	}()

	batch := make([]packet.Packet, 0, maxInboundBatch)
	bufs := make([][]byte, 0, maxInboundBatch)
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
		batch, bufs = batch[:0], bufs[:0]
		e.prep(first, &batch)
		bufs = append(bufs, first)
	drain:
		for len(batch) < maxInboundBatch {
			select {
			case d, ok := <-ch:
				if !ok {
					break drain
				}
				e.prep(d, &batch)
				bufs = append(bufs, d)
			default:
				break drain
			}
		}
		if len(batch) > 0 {
			if err := src.Send(batch); err != nil {
				e.cInErr.Add(1)
			} else {
				e.cInject.Add(uint64(len(batch)))
				e.meter.delivered(batch)
				e.meter.Back.Add(uint64(len(batch)))
			}
		}
		for _, b := range bufs {
			e.bufPool.Put(b[:cap(b)])
		}
	}
}

func (e *Engine) prep(data []byte, batch *[]packet.Packet) {
	if e.rewriter != nil {
		e.rewriter.Inbound(data)
	}
	*batch = append(*batch, packet.Packet{Data: data, Dir: packet.Inbound})
}

type tcpTunnel struct {
	src     packet.Source
	writers []packet.Writer
	ch      chan []byte
	out     chan []byte
	pool    *sync.Pool

	cPush, cDrop, cRead, cWrite, cWriteErr, cOutDrop, cBatches atomic.Uint64

	meter *Meter
}

func (t *tcpTunnel) push(pkt []byte) {
	buf := t.pool.Get().([]byte)
	n := copy(buf, pkt)
	select {
	case t.ch <- buf[:n]:
		t.cPush.Add(1)
	default:
		t.cDrop.Add(1)
		t.pool.Put(buf)
	}
}

func (t *tcpTunnel) ReadPacket(b []byte) (int, error) {
	data, ok := <-t.ch
	if !ok {
		return 0, context.Canceled
	}
	n := copy(b, data)
	t.pool.Put(data[:cap(data)])
	t.cRead.Add(1)
	return n, nil
}

func (t *tcpTunnel) WritePacket(b []byte) ([]byte, error) {
	buf := t.pool.Get().([]byte)
	n := copy(buf, b)
	select {
	case t.out <- buf[:n]:
	default:
		t.cOutDrop.Add(1)
		t.pool.Put(buf)
	}
	return nil, nil
}

const injectGather = 300 * time.Microsecond

func (t *tcpTunnel) injector(ctx context.Context, w packet.Writer) {
	batch := make([]packet.Packet, 0, maxInboundBatch)
	bufs := make([][]byte, 0, maxInboundBatch)
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
		batch, bufs = batch[:0], bufs[:0]
		batch = append(batch, packet.Packet{Data: first, Dir: packet.Inbound})
		bufs = append(bufs, first)

		timer.Reset(injectGather)
	drain:
		for len(batch) < maxInboundBatch {
			select {
			case d, ok := <-t.out:
				if !ok {
					break drain
				}
				batch = append(batch, packet.Packet{Data: d, Dir: packet.Inbound})
				bufs = append(bufs, d)
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
		for _, b := range bufs {
			t.pool.Put(b[:cap(b)])
		}
	}
}

func isTCP(pkt []byte) bool {
	if len(pkt) < 1 {
		return false
	}
	switch pkt[0] >> 4 {
	case 4:
		return len(pkt) >= 20 && pkt[9] == 6
	case 6:
		return len(pkt) >= 40 && pkt[6] == 6
	}
	return false
}

func dstAddr(b []byte) (netip.Addr, bool) {
	if len(b) < 1 {
		return netip.Addr{}, false
	}
	switch b[0] >> 4 {
	case 4:
		if len(b) < 20 {
			return netip.Addr{}, false
		}
		return netip.AddrFrom4([4]byte(b[16:20])), true
	case 6:
		if len(b) < 40 {
			return netip.Addr{}, false
		}
		return netip.AddrFrom16([16]byte(b[24:40])), true
	}
	return netip.Addr{}, false
}

var _ engine.Engine = (*Engine)(nil)

func isDNS(pkt []byte) bool {
	var proto byte
	var rest []byte
	switch {
	case len(pkt) >= 20 && pkt[0]>>4 == 4:
		head := int(pkt[0]&0x0f) * 4
		if head < 20 || len(pkt) < head+8 {
			return false
		}
		proto, rest = pkt[9], pkt[head:]
	case len(pkt) >= 48 && pkt[0]>>4 == 6:
		proto, rest = pkt[6], pkt[40:]
	default:
		return false
	}
	return proto == 17 && binary.BigEndian.Uint16(rest[2:4]) == 53
}

func (e *Engine) CatchDNS(on bool) { e.catchDNS.Store(on) }

func (e *Engine) Direct(fn func(pkt []byte) bool) { e.direct.Store(&fn) }

func (e *Engine) stepsAside(pkt []byte) bool {
	held := e.direct.Load()
	return held != nil && *held != nil && (*held)(pkt)
}

func (e *Engine) Stack() *netstack.Stack { return e.ns.Load() }

func (e *Engine) Mark(fn func(pkt []byte) uint64) { e.mark.Store(&fn) }

func (e *Engine) Loud(on bool) { e.loud = on }

func (e *Engine) carry(tun engine.PacketTunnel, pkt []byte) ([]byte, error) {
	marked, ok := tun.(engine.MarkedTunnel)
	held := e.mark.Load()
	if !ok || held == nil || *held == nil {
		return tun.WritePacket(pkt)
	}
	return marked.WritePacketMarked(pkt, (*held)(pkt))
}

func (e *Engine) Fast(fn func()) {
	if fn == nil {
		return
	}
	e.fast.Store(&fn)
}

func (e *Engine) hurry() func() {
	held := e.fast.Load()
	if held == nil {
		return func() {}
	}
	runtime.LockOSThread()
	(*held)()
	return runtime.UnlockOSThread
}
