package hybrid

import (
	"context"
	"encoding/binary"
	"errors"
	"log"
	"net/netip"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	quic "github.com/quic-go/quic-go"

	"github.com/jaywehosl/qd/internal/ippkt"
	"github.com/jaywehosl/qd/internal/qcli/guard"
	"github.com/jaywehosl/qd/internal/qcli/nat"
	"github.com/jaywehosl/qd/internal/qcli/packet"
	"github.com/jaywehosl/qd/internal/qsrv/server/netstack"
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
	Detour   func(dst netip.Addr) bool
	Mark     func(pkt []byte) uint64
	Loud     bool
	Jumbo    uint16
	Gateway  netip.Addr
	Gateway6 netip.Addr
	Gen      *atomic.Uint64
}

type Engine struct {
	Options
	bufPool sync.Pool
	tcpPool sync.Pool

	flows     sync.Map
	flowCount atomic.Int64

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
	errc := make(chan error, e.Workers+4)

	ms, multi := src.(packet.MultiSource)
	var writer packet.Writer = src
	if multi {
		writer = ms.NewWriter()
	}
	composer, ok := writer.(packet.Composer)
	if !ok {
		return errors.New("hybrid: the packet source cannot compose batches")
	}

	tt := &tcpTunnel{
		out:   make(chan []byte, 4096),
		pool:  &e.tcpPool,
		big:   &e.bufPool,
		meter: e.Meter,
		beat:  startPulse(ctx, errc),
		done:  ctx.Done(),
		quit:  make(chan struct{}),
		sent:  make(chan struct{}),
	}

	detach := e.Stack.Attach(tt)
	go func() {
		defer e.hurry()()
		defer close(tt.sent)
		tt.deliver(e.Stack, composer)
	}()

	defer func() {
		e.Stack.Reset(tt, resetDrain)
		close(tt.quit)
		select {
		case <-tt.sent:
		case <-time.After(resetDrain):
		}
		detach()
	}()

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
			log.Printf("  bridge: inject=%d batches=%d (avg %.1f pkt/syscall) outDrop=%d writeErr=%d tunDrop=%d | datagram DROPPED=%.2f%%",
				tt.cWrite.Load(), tt.cBatches.Load(), avgBatch,
				tt.cOutDrop.Load(), tt.cWriteErr.Load(), sunk, pct)
			log.Printf("  stack: %s", e.Stack.DebugStats())
			bbr, brutal := quic.CongestionSenders()
			log.Printf("  senders: bbr=%d brutal=%d since the client started", bbr, brutal)
		}
	}
}

func (e *Engine) pumpOutbound(ctx context.Context, rd packet.Reader, src packet.Source, tun Tunnel, tt *tcpTunnel, errc chan<- error) {
	beat := startPulse(ctx, errc)
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
				if ippkt.StandIn.Contains(dst) || e.cut(p.Data, tagDirect) {
					reinject = refuse(reinject, p)
					continue
				}
				reinject = append(reinject, *p)
				continue
			}
			if !catch && dst.Is6() && !e.Gateway6.IsValid() {
				reinject = refuse(reinject, p)
				continue
			}
			aside := !catch && e.Detour != nil && ippkt.IsUDP(p.Data) && e.Detour(dst)
			if aside && e.cut(p.Data, tagAside) {
				reinject = refuse(reinject, p)
				continue
			}
			if catch || aside || ippkt.IsTCP(p.Data) {
				e.cTCP.Add(1)
				e.Meter.carried(len(p.Data))
				if e.Jumbo != 0 {
					ippkt.SetMSS(p.Data, e.Jumbo)
				}
				e.Stack.Inject(p.Data)
				continue
			}
			if expired := e.expired(p.Data); expired != nil {
				reinject = append(reinject, packet.Packet{Data: expired, Dir: packet.Inbound})
				continue
			}
			mark := e.mark(p.Data)
			if e.cut(p.Data, mark) {
				reinject = refuse(reinject, p)
				continue
			}
			e.cUDP.Add(1)
			e.Meter.carried(len(p.Data))
			e.NAT.Outbound(p.Data)
			icmp, err := e.carry(tun, p.Data, mark)
			if err != nil {
				e.cWriteErr.Add(1)
				continue
			}
			if len(icmp) > 0 && e.inPieces(tun, p.Data, mark) {
				continue
			}
			if len(icmp) > 0 {
				e.cOversize.Add(1)
				e.NAT.Inbound(icmp)
				reinject = append(reinject, packet.Packet{Data: icmp, Dir: packet.Inbound})
			}
		}
		e.Stack.Flush()
		if len(reinject) > 0 {
			beat.begin()
			err := src.Send(reinject)
			beat.end()
			if err != nil {
				e.cInErr.Add(1)
			}
		}
	}
}

func (e *Engine) pumpInbound(ctx context.Context, src packet.Source, tun Tunnel, errc chan<- error) {
	beat := startPulse(ctx, errc)
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
		beat.begin()
		err := src.Send(batch)
		beat.end()
		if err != nil {
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
	out  chan []byte
	pool *sync.Pool
	big  *sync.Pool

	cWrite, cWriteErr, cOutDrop, cBatches atomic.Uint64

	meter *Meter
	beat  *pulse
	done  <-chan struct{}
	quit  chan struct{}
	sent  chan struct{}
}

func (t *tcpTunnel) give(b []byte) {
	switch cap(b) {
	case tcpSlot:
		t.pool.Put((*[tcpSlot]byte)(b[:tcpSlot]))
	case bufSlot:
		t.big.Put((*[bufSlot]byte)(b[:bufSlot]))
	}
}

func (t *tcpTunnel) ReadPacket([]byte) (int, error) {
	<-t.done
	return 0, context.Canceled
}

func (t *tcpTunnel) WritePacket(b []byte) ([]byte, error) {
	var held []byte
	switch {
	case len(b) > bufSlot:
		held = append([]byte(nil), b...)
	case len(b) > tcpSlot:
		slot := t.big.Get().(*[bufSlot]byte)
		held = slot[:copy(slot[:], b)]
	default:
		slot := t.pool.Get().(*[tcpSlot]byte)
		held = slot[:copy(slot[:], b)]
	}
	select {
	case t.out <- held:
	default:
		t.give(held)
		t.cOutDrop.Add(1)
	}
	return nil, nil
}

func (t *tcpTunnel) deliver(st *netstack.Stack, w packet.Composer) {
	pending := 0
	flush := func() {
		if pending == 0 {
			return
		}
		t.beat.begin()
		err := w.Flush()
		t.beat.end()
		if err != nil {
			t.cWriteErr.Add(1)
		} else {
			t.cWrite.Add(uint64(pending))
		}
		t.cBatches.Add(1)
		pending = 0
	}
	size := 0
	take := func(dst []byte) ([]byte, bool) {
		out, ok := st.Take(dst)
		size = len(out) - len(dst)
		return out, ok
	}
	put := func(d []byte) {
		added, full := w.Compose(func(dst []byte) ([]byte, bool) { return append(dst, d...), true })
		if added {
			pending++
			t.meter.In.Add(1)
			t.meter.BytesIn.Add(uint64(len(d)))
		}
		t.give(d)
		if full {
			flush()
		}
	}

	last := false
	for {
		if !last {
			select {
			case <-st.Ready():
			case d := <-t.out:
				put(d)
			case <-t.quit:
				last = true
			}
		}
		for {
			moved := false
			for {
				added, full := w.Compose(take)
				if !added {
					break
				}
				moved = true
				pending++
				t.meter.In.Add(1)
				t.meter.BytesIn.Add(uint64(size))
				if full {
					flush()
				}
			}
			for more := true; more; {
				select {
				case d := <-t.out:
					put(d)
					moved = true
				default:
					more = false
				}
			}
			flush()
			if !moved {
				break
			}
		}
		if last {
			return
		}
	}
}

func (e *Engine) stepsAside(pkt []byte) bool {
	return e.Direct != nil && e.Direct(pkt)
}

func (e *Engine) mark(pkt []byte) uint64 {
	if e.Mark == nil {
		return 0
	}
	return e.Mark(pkt)
}

const (
	tagDirect uint64 = 1<<62 + iota
	tagAside
	flowsCap = 16384
)

type flowKey struct{ src, dst netip.AddrPort }

type flowHeld struct{ tag, gen uint64 }

func (e *Engine) cut(pkt []byte, tag uint64) bool {
	if e.Gen == nil || !ippkt.IsUDP(pkt) {
		return false
	}
	src, dst, _, ok := ippkt.Flow(pkt)
	if !ok {
		return false
	}
	gen, key := e.Gen.Load(), flowKey{src, dst}
	held, seen := e.flows.Load(key)
	if !seen && e.flowCount.Add(1) > flowsCap {
		e.flows.Clear()
		e.flowCount.Store(1)
	}
	was, _ := held.(flowHeld)
	if seen && was.tag == tag && was.gen == gen {
		return false
	}
	e.flows.Store(key, flowHeld{tag: tag, gen: gen})
	return seen && was.tag != tag && was.gen != gen
}

func refuse(reinject []packet.Packet, p *packet.Packet) []packet.Packet {
	if refusal := ippkt.Refuse(p.Data); refusal != nil {
		reinject = append(reinject, packet.Packet{Data: refusal, Dir: packet.Inbound, IfIndex: p.IfIndex})
	}
	return reinject
}

func (e *Engine) carry(tun Tunnel, pkt []byte, mark uint64) ([]byte, error) {
	marked, ok := tun.(markedTunnel)
	if !ok || e.Mark == nil {
		return tun.WritePacket(pkt)
	}
	return marked.WritePacketMarked(pkt, mark)
}

func (e *Engine) inPieces(tun Tunnel, pkt []byte, mark uint64) bool {
	pieces := ippkt.Fragments4(pkt, pieceSize)
	for _, piece := range pieces {
		if icmp, err := e.carry(tun, piece, mark); err != nil || len(icmp) > 0 {
			return false
		}
	}
	return len(pieces) > 0
}

const pieceSize = 1200

func (e *Engine) hurry() func() {
	if e.Fast == nil {
		return func() {}
	}
	runtime.LockOSThread()
	e.Fast()
	return runtime.UnlockOSThread
}

type pulse struct {
	held atomic.Int64
}

func startPulse(ctx context.Context, errc chan<- error) *pulse {
	p := &pulse{}
	go p.watch(ctx, errc)
	return p
}

func (p *pulse) begin() { p.held.Store(time.Now().UnixNano()) }

func (p *pulse) end() { p.held.Store(0) }

var errStalled = errors.New("the capture stopped taking packets, it is closed so the machine keeps its network")

func (p *pulse) watch(ctx context.Context, errc chan<- error) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		if since := p.held.Load(); since != 0 && time.Since(time.Unix(0, since)) > stallLimit {
			select {
			case errc <- errStalled:
			default:
			}
			return
		}
	}
}

const stallLimit = 5 * time.Second

const resetDrain = 300 * time.Millisecond

const statsEvery = time.Minute

func (e *Engine) expired(pkt []byte) []byte {
	if e.Gateway6.Is6() && len(pkt) >= 48 && pkt[0]>>4 == 6 && pkt[7] <= 1 {
		msg := append([]byte{3, 0, 0, 0, 0, 0, 0, 0}, pkt[:min(len(pkt), 1280-48)]...)
		return ippkt.ICMPv6(e.Gateway6, netip.AddrFrom16([16]byte(pkt[8:24])), 64, msg)
	}
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
