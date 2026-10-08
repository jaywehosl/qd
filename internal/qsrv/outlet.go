package qsrv

import (
	"context"
	"net"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/qd/internal/qsrv/server/netstack"
)

const (
	outletSlot   = 2048
	outletQueue  = 512
	outletBuffer = 2 << 20
)

var outletDrops, linkDrops atomic.Uint64

func Dropped() (outlets, links uint64) { return outletDrops.Load(), linkDrops.Load() }

var outletPool = sync.Pool{New: func() any { return new([outletSlot]byte) }}

type origin struct {
	s    *live
	slot uint32
}

func (o origin) known() bool { return o.s != nil }

type here struct {
	netstack.NetDialer
	from origin
}

func (d here) DialUDP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	if !d.from.known() {
		return d.NetDialer.DialUDP(ctx, dst)
	}
	return d.from.s.reach(d.from.slot, dst)
}

type outlet struct {
	sock *net.UDPConn

	mu    sync.RWMutex
	flows map[netip.AddrPort][]*outFlow
}

func (s *live) reach(slot uint32, dst netip.AddrPort) (net.Conn, error) {
	dst = netip.AddrPortFrom(dst.Addr().Unmap(), dst.Port())

	s.outMu.Lock()
	defer s.outMu.Unlock()

	o := s.outlets[slot]
	if o == nil {
		sock, err := net.ListenUDP("udp", nil)
		if err != nil {
			return nil, err
		}
		sock.SetReadBuffer(outletBuffer)
		o = &outlet{sock: sock, flows: map[netip.AddrPort][]*outFlow{}}
		if s.outlets == nil {
			s.outlets = map[uint32]*outlet{}
		}
		s.outlets[slot] = o
		go o.read()
	}

	f := &outFlow{o: o, dst: dst, in: make(chan []byte, outletQueue), quit: make(chan struct{})}
	f.leave = func() {
		s.outMu.Lock()
		o.mu.Lock()
		o.drop(f)
		last := len(o.flows) == 0
		if last && s.outlets[slot] == o {
			delete(s.outlets, slot)
		}
		o.mu.Unlock()
		s.outMu.Unlock()
		if last {
			o.sock.Close()
		}
	}

	o.mu.Lock()
	o.flows[dst] = append(o.flows[dst], f)
	o.mu.Unlock()
	return f, nil
}

func (o *outlet) drop(f *outFlow) {
	held := o.flows[f.dst]
	for i, one := range held {
		if one != f {
			continue
		}
		if held = append(held[:i:i], held[i+1:]...); len(held) == 0 {
			delete(o.flows, f.dst)
		} else {
			o.flows[f.dst] = held
		}
		return
	}
}

func (o *outlet) read() {
	big := make([]byte, 65535)
	for {
		n, from, err := o.sock.ReadFromUDPAddrPort(big)
		if err != nil {
			return
		}
		from = netip.AddrPortFrom(from.Addr().Unmap(), from.Port())

		o.mu.RLock()
		var f *outFlow
		if held := o.flows[from]; len(held) > 0 {
			f = held[len(held)-1]
		}
		o.mu.RUnlock()
		if f == nil {
			continue
		}

		var pkt []byte
		if n <= outletSlot {
			pkt = outletPool.Get().(*[outletSlot]byte)[:n]
		} else {
			pkt = make([]byte, n)
		}
		copy(pkt, big[:n])
		select {
		case f.in <- pkt:
		default:
			outletDrops.Add(1)
			giveBack(pkt)
		}
	}
}

func giveBack(pkt []byte) {
	if cap(pkt) == outletSlot {
		outletPool.Put((*[outletSlot]byte)(pkt[:outletSlot]))
	}
}

type outFlow struct {
	o     *outlet
	dst   netip.AddrPort
	in    chan []byte
	quit  chan struct{}
	once  sync.Once
	leave func()
	until atomic.Int64
}

func (f *outFlow) Read(p []byte) (int, error) {
	var late <-chan time.Time
	if at := f.until.Load(); at != 0 {
		wait := time.Until(time.Unix(0, at))
		if wait <= 0 {
			return 0, os.ErrDeadlineExceeded
		}
		timer := time.NewTimer(wait)
		defer timer.Stop()
		late = timer.C
	}
	select {
	case pkt := <-f.in:
		n := copy(p, pkt)
		giveBack(pkt)
		return n, nil
	case <-f.quit:
		return 0, net.ErrClosed
	case <-late:
		return 0, os.ErrDeadlineExceeded
	}
}

func (f *outFlow) Write(p []byte) (int, error) {
	select {
	case <-f.quit:
		return 0, net.ErrClosed
	default:
	}
	return f.o.sock.WriteToUDPAddrPort(p, f.dst)
}

func (f *outFlow) Close() error {
	f.once.Do(func() {
		close(f.quit)
		f.leave()
	})
	return nil
}

func (f *outFlow) LocalAddr() net.Addr  { return f.o.sock.LocalAddr() }
func (f *outFlow) RemoteAddr() net.Addr { return net.UDPAddrFromAddrPort(f.dst) }

func (f *outFlow) SetDeadline(t time.Time) error { return f.SetReadDeadline(t) }

func (f *outFlow) SetReadDeadline(t time.Time) error {
	if t.IsZero() {
		f.until.Store(0)
	} else {
		f.until.Store(t.UnixNano())
	}
	return nil
}

func (f *outFlow) SetWriteDeadline(time.Time) error { return nil }
