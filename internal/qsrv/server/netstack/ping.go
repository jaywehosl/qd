package netstack

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"github.com/jaywehosl/qd/internal/ippkt"
)

type Echo struct {
	From            netip.Addr
	Type, Code, TTL uint8
}

type Pinger interface {
	Ping(ctx context.Context, dst netip.Addr, ttl uint8, payload []byte) (Echo, error)
}

const (
	EchoWait  = 5 * time.Second
	maxProbes = 1024
	maxEcho   = 1472
	minMTU6   = 1280
)

func isEcho4(pkt []byte) bool {
	if len(pkt) < 28 || pkt[0]>>4 != 4 || pkt[9] != 1 {
		return false
	}
	ihl := int(pkt[0]&0x0F) * 4
	return ihl >= 20 && len(pkt) >= ihl+8 && pkt[ihl] == 8 && pkt[6]&0x1F == 0 && pkt[7] == 0
}

func isEcho6(pkt []byte) bool {
	return len(pkt) >= 48 && pkt[0]>>4 == 6 && pkt[6] == 58 && pkt[40] == 128 && pkt[41] == 0
}

func (s *Stack) echo(t Tunnel, pkt []byte) {
	ihl := int(pkt[0]&0x0F) * 4
	ctx, cancel := context.WithTimeout(context.Background(), EchoWait)
	defer cancel()

	got, err := s.pinger.Ping(ctx, netip.AddrFrom4([4]byte(pkt[16:20])), pkt[8], pkt[ihl+8:])
	if err != nil || !got.From.Is4() {
		return
	}

	var msg []byte
	if got.Type == 0 {
		msg = append([]byte{0, 0, 0, 0}, pkt[ihl+4:]...)
	} else {
		msg = append([]byte{got.Type, got.Code, 0, 0, 0, 0, 0, 0}, pkt[:ihl+8]...)
	}
	binary.BigEndian.PutUint16(msg[2:], ippkt.Checksum(msg))

	out := make([]byte, 20+len(msg))
	out[0], out[8], out[9] = 0x45, got.TTL, 1
	if out[8] == 0 {
		out[8] = 64
	}
	binary.BigEndian.PutUint16(out[2:], uint16(len(out)))
	from := got.From.As4()
	copy(out[12:16], from[:])
	copy(out[16:20], pkt[12:16])
	binary.BigEndian.PutUint16(out[10:], ippkt.Checksum(out[:20]))
	copy(out[20:], msg)

	s.wmu.Lock()
	t.WritePacket(out)
	s.wmu.Unlock()
}

func (s *Stack) echo6(t Tunnel, pkt []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), EchoWait)
	defer cancel()

	got, err := s.pinger.Ping(ctx, netip.AddrFrom16([16]byte(pkt[24:40])), pkt[7], pkt[48:])
	if err != nil || !got.From.Is6() {
		return
	}

	var msg []byte
	switch got.Type {
	case 129:
		msg = append([]byte{129, 0, 0, 0}, pkt[44:]...)
	case 1, 3:
		quote := pkt[:min(len(pkt), minMTU6-48)]
		msg = append([]byte{got.Type, got.Code, 0, 0, 0, 0, 0, 0}, quote...)
	default:
		return
	}

	out := ippkt.ICMPv6(got.From, netip.AddrFrom16([16]byte(pkt[8:24])), got.TTL, msg)

	s.wmu.Lock()
	t.WritePacket(out)
	s.wmu.Unlock()
}

func (NetDialer) Ping(ctx context.Context, dst netip.Addr, ttl uint8, payload []byte) (Echo, error) {
	dst = dst.Unmap()
	p, err := sharedProber(dst.Is6())
	if err != nil {
		return Echo{}, err
	}
	return p.ping(ctx, dst, ttl, payload)
}

type probeOnce struct {
	once sync.Once
	p    *prober
	err  error
}

var probe4, probe6 probeOnce

func sharedProber(six bool) (*prober, error) {
	if six {
		probe6.once.Do(func() { probe6.p, probe6.err = open6() })
		return probe6.p, probe6.err
	}
	probe4.once.Do(func() { probe4.p, probe4.err = open4() })
	return probe4.p, probe4.err
}

func open4() (*prober, error) {
	c, err := net.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return nil, err
	}
	pc := ipv4.NewPacketConn(c)
	pc.SetControlMessage(ipv4.FlagTTL, true)
	p := newProber(8, func(msg []byte, dst netip.Addr, ttl uint8) error {
		if err := pc.SetTTL(int(ttl)); err != nil {
			return err
		}
		_, err := pc.WriteTo(msg, nil, &net.IPAddr{IP: dst.AsSlice()})
		return err
	})
	go func() {
		buf := make([]byte, 65536)
		for {
			n, cm, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			ttl := 0
			if cm != nil {
				ttl = cm.TTL
			}
			p.heard(buf[:n], from, ttl)
		}
	}()
	return p, nil
}

func open6() (*prober, error) {
	c, err := net.ListenPacket("ip6:ipv6-icmp", "::")
	if err != nil {
		return nil, err
	}
	pc := ipv6.NewPacketConn(c)
	pc.SetControlMessage(ipv6.FlagHopLimit, true)
	p := newProber(128, func(msg []byte, dst netip.Addr, ttl uint8) error {
		if err := pc.SetHopLimit(int(ttl)); err != nil {
			return err
		}
		_, err := pc.WriteTo(msg, nil, &net.IPAddr{IP: dst.AsSlice()})
		return err
	})
	go func() {
		buf := make([]byte, 65536)
		for {
			n, cm, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			hop := 0
			if cm != nil {
				hop = cm.HopLimit
			}
			p.heard(buf[:n], from, hop)
		}
	}()
	return p, nil
}

type prober struct {
	send    func(msg []byte, dst netip.Addr, ttl uint8) error
	request byte
	id      uint16
	wmu     sync.Mutex

	mu   sync.Mutex
	seq  uint16
	wait map[uint16]chan Echo
}

func newProber(request byte, send func([]byte, netip.Addr, uint8) error) *prober {
	return &prober{send: send, request: request, id: uint16(os.Getpid()), wait: map[uint16]chan Echo{}}
}

func (p *prober) ping(ctx context.Context, dst netip.Addr, ttl uint8, payload []byte) (Echo, error) {
	if len(payload) > maxEcho {
		payload = payload[:maxEcho]
	}
	if ttl == 0 {
		ttl = 64
	}

	ch := make(chan Echo, 1)
	p.mu.Lock()
	if len(p.wait) >= maxProbes {
		p.mu.Unlock()
		return Echo{}, errors.New("ping: too many probes in flight")
	}
	p.seq++
	for p.wait[p.seq] != nil {
		p.seq++
	}
	seq := p.seq
	p.wait[seq] = ch
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.wait, seq)
		p.mu.Unlock()
	}()

	msg := make([]byte, 8+len(payload))
	msg[0] = p.request
	binary.BigEndian.PutUint16(msg[4:], p.id)
	binary.BigEndian.PutUint16(msg[6:], seq)
	copy(msg[8:], payload)
	if p.request == 8 {
		binary.BigEndian.PutUint16(msg[2:], ippkt.Checksum(msg))
	}

	p.wmu.Lock()
	err := p.send(msg, dst, ttl)
	p.wmu.Unlock()
	if err != nil {
		return Echo{}, err
	}
	select {
	case got := <-ch:
		return got, nil
	case <-ctx.Done():
		return Echo{}, ctx.Err()
	}
}

func (p *prober) heard(b []byte, from net.Addr, ttl int) {
	if len(b) < 8 {
		return
	}
	six := p.request == 128
	at, inner := -1, b[8:]
	switch {
	case !six && b[0] == 0, six && b[0] == 129:
		at, inner = 0, b
	case !six && (b[0] == 3 || b[0] == 11):
		if len(inner) >= 28 && inner[9] == 1 && inner[0]&0x0F >= 5 {
			at = int(inner[0]&0x0F) * 4
		}
	case six && (b[0] == 1 || b[0] == 3):
		if len(inner) >= 48 && inner[6] == 58 {
			at = 40
		}
	}
	if at < 0 || len(inner) < at+8 || (at > 0 && inner[at] != p.request) {
		return
	}
	id, seq := binary.BigEndian.Uint16(inner[at+4:]), binary.BigEndian.Uint16(inner[at+6:])
	if id != p.id {
		return
	}
	ip, ok := from.(*net.IPAddr)
	if !ok {
		return
	}
	addr, ok := netip.AddrFromSlice(ip.IP)
	if !ok {
		return
	}
	if ttl <= 0 || ttl > 255 {
		ttl = 0
	}
	p.mu.Lock()
	ch := p.wait[seq]
	p.mu.Unlock()
	if ch != nil {
		select {
		case ch <- Echo{From: addr.Unmap(), Type: b[0], Code: b[1], TTL: uint8(ttl)}:
		default:
		}
	}
}
