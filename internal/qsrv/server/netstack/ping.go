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

	"github.com/jaywehosl/quic-diver/internal/ippkt"
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
)

func isEcho4(pkt []byte) bool {
	if len(pkt) < 28 || pkt[0]>>4 != 4 || pkt[9] != 1 {
		return false
	}
	ihl := int(pkt[0]&0x0F) * 4
	return ihl >= 20 && len(pkt) >= ihl+8 && pkt[ihl] == 8 && pkt[6]&0x1F == 0 && pkt[7] == 0
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

func (NetDialer) Ping(ctx context.Context, dst netip.Addr, ttl uint8, payload []byte) (Echo, error) {
	p, err := sharedProber()
	if err != nil {
		return Echo{}, err
	}
	return p.ping(ctx, dst, ttl, payload)
}

var probe struct {
	once sync.Once
	p    *prober
	err  error
}

func sharedProber() (*prober, error) {
	probe.once.Do(func() {
		c, err := net.ListenPacket("ip4:icmp", "0.0.0.0")
		if err != nil {
			probe.err = err
			return
		}
		pc := ipv4.NewPacketConn(c)
		pc.SetControlMessage(ipv4.FlagTTL, true)
		probe.p = &prober{conn: pc, id: uint16(os.Getpid()), wait: map[uint16]chan Echo{}}
		go probe.p.read()
	})
	return probe.p, probe.err
}

type prober struct {
	conn *ipv4.PacketConn
	id   uint16
	wmu  sync.Mutex

	mu   sync.Mutex
	seq  uint16
	wait map[uint16]chan Echo
}

func (p *prober) ping(ctx context.Context, dst netip.Addr, ttl uint8, payload []byte) (Echo, error) {
	dst = dst.Unmap()
	if !dst.Is4() {
		return Echo{}, errors.New("ping: ipv4 only")
	}
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
	msg[0] = 8
	binary.BigEndian.PutUint16(msg[4:], p.id)
	binary.BigEndian.PutUint16(msg[6:], seq)
	copy(msg[8:], payload)
	binary.BigEndian.PutUint16(msg[2:], ippkt.Checksum(msg))

	p.wmu.Lock()
	err := p.conn.SetTTL(int(ttl))
	if err == nil {
		_, err = p.conn.WriteTo(msg, nil, &net.IPAddr{IP: dst.AsSlice()})
	}
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

func (p *prober) read() {
	buf := make([]byte, 65536)
	for {
		n, cm, from, err := p.conn.ReadFrom(buf)
		if err != nil {
			return
		}
		b := buf[:n]
		if len(b) < 8 {
			continue
		}
		var id, seq uint16
		switch b[0] {
		case 0:
			id, seq = binary.BigEndian.Uint16(b[4:]), binary.BigEndian.Uint16(b[6:])
		case 3, 11:
			inner := b[8:]
			if len(inner) < 28 {
				continue
			}
			ihl := int(inner[0]&0x0F) * 4
			if ihl < 20 || len(inner) < ihl+8 || inner[9] != 1 || inner[ihl] != 8 {
				continue
			}
			id, seq = binary.BigEndian.Uint16(inner[ihl+4:]), binary.BigEndian.Uint16(inner[ihl+6:])
		default:
			continue
		}
		if id != p.id {
			continue
		}
		ip, ok := from.(*net.IPAddr)
		if !ok {
			continue
		}
		addr, ok := netip.AddrFromSlice(ip.IP)
		if !ok {
			continue
		}
		p.mu.Lock()
		ch := p.wait[seq]
		p.mu.Unlock()
		if ch != nil {
			select {
			case ch <- Echo{From: addr.Unmap(), Type: b[0], Code: b[1], TTL: replyTTL(cm)}:
			default:
			}
		}
	}
}

func replyTTL(cm *ipv4.ControlMessage) uint8 {
	if cm == nil || cm.TTL <= 0 || cm.TTL > 255 {
		return 0
	}
	return uint8(cm.TTL)
}
