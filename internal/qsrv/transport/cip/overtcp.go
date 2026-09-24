package cip

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"sync/atomic"
	"syscall"

	"golang.org/x/net/http2"

	"github.com/jaywehosl/quic-diver/internal/ippkt"
	"github.com/jaywehosl/quic-diver/internal/qsrv"
	"github.com/jaywehosl/quic-diver/internal/roads"
)

type Over struct {
	conn   net.Conn
	cc     *http2.ClientConn
	auth   string
	token  string
	device string
	given  atomic.Pointer[netip.Prefix]

	dgramMu sync.Mutex
	dgramW  *io.PipeWriter
	dgramR  io.ReadCloser
}

func ReachH2(ctx context.Context, endpoint string, keep func(fd uintptr)) (net.Conn, *http2.ClientConn, error) {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, nil, err
	}

	dialer := &net.Dialer{}
	if keep != nil {
		dialer.Control = func(_, _ string, rc syscall.RawConn) error {
			return rc.Control(keep)
		}
	}
	raw, err := roads.ReachTCP(ctx, dialer, endpoint)
	if err != nil {
		return nil, nil, err
	}

	held := tls.Client(raw, &tls.Config{ServerName: host, NextProtos: []string{"h2"}})
	if err := held.HandshakeContext(ctx); err != nil {
		raw.Close()
		return nil, nil, err
	}
	if state := held.ConnectionState(); state.NegotiatedProtocol != "h2" {
		held.Close()
		return nil, nil, fmt.Errorf("the node offered %q, not h2", state.NegotiatedProtocol)
	}

	cc, err := (&http2.Transport{}).NewClientConn(held)
	if err != nil {
		held.Close()
		return nil, nil, err
	}
	return held, cc, nil
}

func DialOver(ctx context.Context, endpoint, token, device, route, authURL string, keep func(fd uintptr)) (*Over, error) {
	conn, cc, err := ReachH2(ctx, endpoint, keep)
	if err != nil {
		return nil, err
	}

	over := &Over{conn: conn, cc: cc, auth: authURL, token: token, device: device}
	if err := over.Steer(ctx, route); err != nil {
		over.Close()
		return nil, err
	}
	if err := over.openDatagram(endpoint, route); err != nil {
		over.Close()
		return nil, err
	}
	return over, nil
}

func (o *Over) openDatagram(endpoint, route string) error {
	pr, pw := io.Pipe()
	req, err := http.NewRequest(http.MethodPost, "https://"+endpoint+qsrv.IPOverTCPPath, pr)
	if err != nil {
		pw.Close()
		return err
	}
	sign(req, o.token, o.device, route)

	resp, err := o.cc.RoundTrip(req)
	if err != nil {
		pw.Close()
		return fmt.Errorf("datagram channel: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		pw.Close()
		return fmt.Errorf("datagram channel refused: %d", resp.StatusCode)
	}
	o.dgramW = pw
	o.dgramR = resp.Body
	return nil
}

func (o *Over) H2Conn() *http2.ClientConn { return o.cc }

func (o *Over) Steer(ctx context.Context, route string) error {
	header, err := greet(ctx, o.cc, o.token, o.device, route, o.auth)
	if err != nil {
		return err
	}
	if given, err := netip.ParsePrefix(header.Get(qsrv.HeaderAddr)); err == nil {
		o.given.Store(&given)
	}
	return nil
}

func (o *Over) Ask(ctx context.Context, route string) error {
	if !o.Alive() {
		return fmt.Errorf("the session is closed")
	}
	return o.Steer(ctx, route)
}

func (o *Over) Alive() bool { return o.cc.CanTakeNewRequest() }

func (o *Over) Close() error {
	if o.dgramW != nil {
		o.dgramW.Close()
	}
	if o.dgramR != nil {
		o.dgramR.Close()
	}
	return o.conn.Close()
}

func (o *Over) LocalPrefixes(context.Context) ([]netip.Prefix, error) {
	if held := o.given.Load(); held != nil {
		return []netip.Prefix{*held}, nil
	}
	return nil, fmt.Errorf("the node named no address for this path")
}

func (o *Over) ReadPacket(b []byte) (int, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(o.dgramR, hdr[:]); err != nil {
		return 0, err
	}
	n := int(binary.BigEndian.Uint16(hdr[:]))
	if n > len(b) {
		return 0, fmt.Errorf("packet %d over buffer %d", n, len(b))
	}
	if _, err := io.ReadFull(o.dgramR, b[:n]); err != nil {
		return 0, err
	}
	return n, nil
}

func (o *Over) WritePacket(b []byte) ([]byte, error) {
	switch {
	case len(b) >= 20 && b[0]>>4 == 4:
		if b[8] <= 1 {
			return nil, fmt.Errorf("cip: datagram TTL too small: %d", b[8])
		}
		b[8]--
		b[10], b[11] = 0, 0
		binary.BigEndian.PutUint16(b[10:], ippkt.Checksum(b[:int(b[0]&0x0F)*4]))
	case len(b) >= 40 && b[0]>>4 == 6:
		if b[7] <= 1 {
			return nil, fmt.Errorf("cip: datagram hop limit too small: %d", b[7])
		}
		b[7]--
	}
	frame := make([]byte, 2+len(b))
	binary.BigEndian.PutUint16(frame, uint16(len(b)))
	copy(frame[2:], b)
	o.dgramMu.Lock()
	_, err := o.dgramW.Write(frame)
	o.dgramMu.Unlock()
	return nil, err
}

func (o *Over) DatagramLimit() int { return 1280 }

func (o *Over) Migrate(context.Context, *net.UDPAddr) error {
	return fmt.Errorf("a tcp path does not migrate")
}
