package qsrv

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync/atomic"
	"time"

	connectip "github.com/quic-go/connect-ip-go"
	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"github.com/jaywehosl/quic-diver/internal/qsrv/server/netstack"
)

var wholeInternet = []connectip.IPRoute{
	{StartIP: netip.MustParseAddr("0.0.0.0"), EndIP: netip.MustParseAddr("255.255.255.255")},
}

func (n *Node) serveConnectIP(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		grant, ok := n.carrier(r)
		if !ok {
			n.refused.Add(1)
			n.site.ServeHTTP(w, r)
			return
		}

		req, err := connectip.ParseRequest(r, n.tmpl)
		if err != nil {
			n.site.ServeHTTP(w, r)
			return
		}

		conn, err := n.proxy.Proxy(w, req)
		if err != nil {
			return
		}

		go n.carry(ctx, conn, sessionOf(r.Context()).quic(), grant, n.routeFor(r))
	}
}

func routeOf(r *http.Request) string { return r.Header.Get(HeaderRoute) }

func (n *Node) routeFor(r *http.Request) string {
	route := routeOf(r)
	if route == "" {
		route = sessionOf(r.Context()).heading()
	}
	return settled(route)
}

func (s *live) wentUp(n int) {
	s.up.Add(uint64(n))
	s.pktUp.Add(1)
	s.lastSeen.Store(time.Now().Unix())
}

func (s *live) cameDown(n int) {
	s.down.Add(uint64(n))
	s.pktDown.Add(1)
	s.lastSeen.Store(time.Now().Unix())
}

func (n *Node) carry(ctx context.Context, conn *connectip.Conn, qc *quic.Conn, grant Grant, route string) {
	address, err := n.pool.take(grant.Seat)
	if err != nil {
		conn.Close()
		return
	}

	if err := conn.AssignAddresses(ctx, []netip.Prefix{address}); err != nil {
		n.pool.give(address)
		conn.Close()
		return
	}
	if err := conn.AdvertiseRoute(ctx, wholeInternet); err != nil {
		n.pool.give(address)
		conn.Close()
		return
	}

	s := newLive(grant, address, "", route)
	s.conn = qc
	defer conn.Close()

	n.runStack(ctx, s, counted{conn: conn, s: s})
}

func (n *Node) runStack(ctx context.Context, s *live, tun netstack.Tunnel) {
	n.mu.Lock()
	if was := n.held[s.grant.Seat]; was != nil {
		n.pool.give(was.address)
	}
	n.held[s.grant.Seat] = s
	n.mu.Unlock()

	defer func() {
		n.mu.Lock()
		if n.held[s.grant.Seat] == s {
			delete(n.held, s.grant.Seat)
		}
		n.mu.Unlock()
		n.pool.give(s.address)
		n.links.forget(s.grant.Seat)
	}()

	stack, err := netstack.New(steered{node: n, grant: s.grant, s: s, hops: defaultHops}, n.Tunables().MTU)
	if err != nil {
		return
	}
	stack.OnFlow(s.holdFlow)
	defer stack.Reset(tun, exitDrain)

	stack.Run(ctx, tun)
}

const exitDrain = 200 * time.Millisecond

type streamTun struct {
	r io.Reader
	w io.Writer
}

func (t *streamTun) ReadPacket(b []byte) (int, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(t.r, hdr[:]); err != nil {
		return 0, err
	}
	n := int(binary.BigEndian.Uint16(hdr[:]))
	if n > len(b) {
		return 0, fmt.Errorf("stream tun: packet %d over buffer %d", n, len(b))
	}
	if _, err := io.ReadFull(t.r, b[:n]); err != nil {
		return 0, err
	}
	return n, nil
}

func (t *streamTun) WritePacket(b []byte) ([]byte, error) {
	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(len(b)))
	if _, err := t.w.Write(hdr[:]); err != nil {
		return nil, err
	}
	if _, err := t.w.Write(b); err != nil {
		return nil, err
	}
	if f, ok := t.w.(http.Flusher); ok {
		f.Flush()
	}
	return nil, nil
}

type countedTun struct {
	tun netstack.Tunnel
	s   *live
}

func (c countedTun) ReadPacket(b []byte) (int, error) {
	n, err := c.tun.ReadPacket(b)
	if err == nil {
		c.s.wentUp(n)
	}
	return n, err
}

func (c countedTun) WritePacket(b []byte) ([]byte, error) {
	icmp, err := c.tun.WritePacket(b)
	if err == nil {
		c.s.cameDown(len(b))
	}
	return icmp, err
}

func (n *Node) serveIPOverTCP(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		grant, ok := n.carrier(r)
		if !ok {
			n.refused.Add(1)
			n.site.ServeHTTP(w, r)
			return
		}

		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		n.carryStream(ctx, &streamTun{r: r.Body, w: flushWriter{w}}, grant, r.RemoteAddr, n.routeFor(r))
	}
}

func (n *Node) carryStream(ctx context.Context, tun netstack.Tunnel, grant Grant, peer, route string) {
	address, err := n.pool.take(grant.Seat)
	if err != nil {
		return
	}

	s := newLive(grant, address, peer, route)
	s.stream = true

	n.runStack(ctx, s, countedTun{tun: tun, s: s})
}

type counted struct {
	conn *connectip.Conn
	s    *live
}

func (c counted) ReadPacket(b []byte) (int, error) {
	n, mark, err := c.conn.ReadPacketMarked(b)
	if err == nil {
		c.s.wentUp(n)
		c.s.noteMark(b[:n], mark)
	}
	return n, err
}

func (c counted) WritePacket(b []byte) ([]byte, error) {
	icmp, err := c.conn.WritePacket(b)
	if err == nil {
		c.s.cameDown(len(b))
	}
	return icmp, err
}

func (n *Node) dialerFor(ctx context.Context, grant Grant, route string, hops int) netstack.Dialer {
	local := netstack.NetDialer{}

	if route == "" || hops <= 0 {
		return local
	}
	if route == n.cfg.SelfID || route == n.cfg.SelfTag {
		return local
	}
	if !grant.AllowExit {
		return refusing{why: fmt.Errorf("this subscription may not take an exit")}
	}

	won, endpoint, err := n.raceExit(ctx, route, grant.Seat, grant.Session)
	if err != nil {
		return refusing{why: err}
	}

	n.transits.Add(1)
	return chained{cc: won, ls: n.links, endpoint: endpoint, seat: grant.Seat, hops: hops - 1}
}

type refusing struct{ why error }

func (d refusing) DialTCP(context.Context, netip.AddrPort) (net.Conn, error) {
	return nil, d.why
}

func (d refusing) DialUDP(context.Context, netip.AddrPort) (net.Conn, error) {
	return nil, d.why
}

func (n *Node) serveConnect(w http.ResponseWriter, r *http.Request) {
	grant, ok := n.carrier(r)
	if !ok {
		n.refused.Add(1)
		n.site.ServeHTTP(w, r)
		return
	}

	route := n.routeFor(r)

	dst, err := netip.ParseAddrPort(r.Host)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if dst.Addr().Is6() && !dst.Addr().Is4In6() && !HoldsV6() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if n.stale(dst) {
		w.WriteHeader(http.StatusGone)
		return
	}
	dst = n.behind(dst)

	hops := defaultHops
	if raw := r.Header.Get(HeaderHops); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			hops = v
		}
	}

	dialCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	dialer := n.dialerFor(dialCtx, grant, route, hops)
	s := n.counting(grant, r, route)

	if r.Header.Get(HeaderProto) == "icmp" {
		n.servePing(w, r, dialer, dst.Addr())
		cancel()
		return
	}

	if r.Header.Get(HeaderProto) == "udp" {
		out, err := dialer.DialUDP(dialCtx, dst)
		cancel()
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if hs, ok := w.(http3.HTTPStreamer); ok && r.Header.Get(HeaderDgram) == "1" {
			n.relayDatagrams(w, hs, out, s)
			return
		}
		n.relayPackets(w, r, out, s)
		return
	}

	out, err := dialer.DialTCP(dialCtx, dst)
	cancel()
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer out.Close()

	if tcp, ok := out.(*net.TCPConn); ok {
		tcp.SetNoDelay(true)
	}

	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	done := make(chan struct{})
	go func() {
		io.Copy(tallied(out, s, true), r.Body)
		if cw, ok := out.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		}
		close(done)
	}()

	io.Copy(tallied(flushWriter{w}, s, false), out)
	<-done
}

type flushWriter struct{ w http.ResponseWriter }

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if f, ok := fw.w.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}

type steered struct {
	node  *Node
	grant Grant
	s     *live
	hops  int
}

func (d steered) DialTCP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	dst = d.node.behind(dst)
	return d.node.dialerFor(ctx, d.grant, d.route(ctx), d.hops).DialTCP(ctx, dst)
}

func (d steered) DialUDP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	dst = d.node.behind(dst)
	return d.node.dialerFor(ctx, d.grant, d.route(ctx), d.hops).DialUDP(ctx, dst)
}

func (d steered) route(ctx context.Context) string {
	if flow, ok := netstack.FlowOf(ctx); ok && d.s.markOf(flow.Src.Port()) == MarkEgress {
		return AnyExit
	}
	return d.s.heading()
}

func (n *Node) relayPackets(w http.ResponseWriter, r *http.Request, out net.Conn, s *live) {
	defer out.Close()

	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	var lastOut atomic.Int64
	lastOut.Store(time.Now().UnixNano())

	go func() {
		defer out.Close()
		var size [2]byte
		buf := make([]byte, 65535)
		for {
			if _, err := io.ReadFull(r.Body, size[:]); err != nil {
				return
			}
			want := int(binary.BigEndian.Uint16(size[:]))
			if _, err := io.ReadFull(r.Body, buf[:want]); err != nil {
				return
			}
			if _, err := out.Write(buf[:want]); err != nil {
				return
			}
			lastOut.Store(time.Now().UnixNano())
			if s != nil {
				s.wentUp(want)
			}
		}
	}()

	var head [2]byte
	buf := make([]byte, 65535)
	for {
		out.SetReadDeadline(time.Now().Add(flowQuiet))
		read, err := out.Read(buf)
		if err != nil {
			if quiet, ok := err.(net.Error); ok && quiet.Timeout() &&
				time.Since(time.Unix(0, lastOut.Load())) < flowQuiet {
				continue
			}
			break
		}
		binary.BigEndian.PutUint16(head[:], uint16(read))
		if _, err := w.Write(head[:]); err != nil {
			break
		}
		if _, err := w.Write(buf[:read]); err != nil {
			break
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if s != nil {
			s.cameDown(read)
		}
	}
}

const flowQuiet = 60 * time.Second

type tally struct {
	to   io.Writer
	sum  *atomic.Uint64
	seen *atomic.Int64
}

func (t tally) Write(p []byte) (int, error) {
	n, err := t.to.Write(p)
	if n > 0 {
		t.sum.Add(uint64(n))
		t.seen.Store(time.Now().Unix())
	}
	return n, err
}

func tallied(to io.Writer, s *live, up bool) io.Writer {
	if s == nil {
		return to
	}
	sum := &s.down
	if up {
		sum = &s.up
	}
	return tally{to: to, sum: sum, seen: &s.lastSeen}
}
