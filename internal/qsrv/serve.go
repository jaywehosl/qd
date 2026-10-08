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

	"github.com/jaywehosl/qd/internal/ippkt"
	"github.com/jaywehosl/qd/internal/qsrv/server/netstack"
)

var wholeInternet = []connectip.IPRoute{
	{StartIP: netip.MustParseAddr("0.0.0.0"), EndIP: netip.MustParseAddr("255.255.255.255")},
	{StartIP: netip.IPv6Unspecified(), EndIP: netip.MustParseAddr("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")},
}

func routesFor(addresses []netip.Prefix) []connectip.IPRoute {
	if len(addresses) > 1 {
		return wholeInternet
	}
	return wholeInternet[:1]
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
	address, early := n.pool.takeOwn(grant.Seat)
	if !early {
		if !settles(qc) {
			conn.Close()
			return
		}
		var err error
		if address, err = n.pool.take(grant.Seat); err != nil {
			conn.Close()
			return
		}
	}

	addresses := carried(address)
	if err := conn.AssignAddresses(ctx, addresses); err != nil {
		n.pool.give(address)
		conn.Close()
		return
	}
	if err := conn.AdvertiseRoute(ctx, routesFor(addresses)); err != nil {
		n.pool.give(address)
		conn.Close()
		return
	}
	if !settles(qc) {
		n.pool.give(address)
		conn.Close()
		return
	}

	s := newLive(grant, address, "", route)
	s.conn = qc
	s.marked = true
	s.end = func() { conn.Close() }
	defer conn.Close()

	n.runStack(ctx, s, counted{conn: conn, s: s})
}

func (n *Node) runStack(ctx context.Context, s *live, tun netstack.Tunnel) {
	seat := s.grant.Seat
	n.mu.Lock()
	was := n.held[seat]
	s.under = was
	n.held[seat] = s
	n.mu.Unlock()

	defer func() {
		s.over.Store(true)
		n.mu.Lock()
		current := n.held[seat] == s
		s.under = nil
		back := current && was != nil && was.end != nil && !was.over.Load()
		switch {
		case back:
			n.held[seat] = was
		case current:
			delete(n.held, seat)
		}
		n.mu.Unlock()
		n.pool.give(s.address)
		if current && !back {
			n.links.forget(seat)
		}
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
	r     io.Reader
	w     io.Writer
	marks int
	note  func(pkt []byte, mark uint64)
}

func (t *streamTun) ReadPacket(b []byte) (int, error) {
	var hdr [3]byte
	if _, err := io.ReadFull(t.r, hdr[:2+t.marks]); err != nil {
		return 0, err
	}
	n := int(binary.BigEndian.Uint16(hdr[:])) - t.marks
	if n < 0 || n > len(b) {
		return 0, fmt.Errorf("stream tun: packet %d over buffer %d", n, len(b))
	}
	if _, err := io.ReadFull(t.r, b[:n]); err != nil {
		return 0, err
	}
	if t.note != nil {
		t.note(b[:n], uint64(hdr[2]))
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

		tun := &streamTun{r: r.Body, w: flushWriter{w}}
		if r.Header.Get(HeaderMarks) == "1" {
			tun.marks = 1
			w.Header().Set(HeaderMarks, "1")
		}
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		n.carryStream(ctx, tun, func() { r.Body.Close() }, grant, r.RemoteAddr, n.routeFor(r))
	}
}

func (n *Node) carryStream(ctx context.Context, tun *streamTun, end func(), grant Grant, peer, route string) {
	address, err := n.pool.take(grant.Seat)
	if err != nil {
		return
	}

	s := newLive(grant, address, peer, route)
	s.stream = true
	s.end = end
	if tun.marks > 0 {
		s.marked, tun.note = true, s.noteMark
	}

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
	if err == nil && len(icmp) > 0 {
		for _, piece := range ippkt.Fragments4(b, pieceSize) {
			if _, err = c.conn.WritePacket(piece); err != nil {
				break
			}
			icmp = nil
		}
	}
	if err == nil {
		c.s.cameDown(len(b))
	}
	return icmp, err
}

const pieceSize = 1200

func (n *Node) dialerFor(ctx context.Context, grant Grant, route string, hops int, from origin) netstack.Dialer {
	local := here{from: from}

	if hops <= 0 {
		return local
	}
	if route == "" {
		if grant.Steer && n.steer.any() {
			return steering{node: n, grant: grant, hops: hops, local: local}
		}
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
	return chained{cc: won, ls: n.links, endpoint: endpoint, seat: grant.Seat, hops: hops - 1, from: from}
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
	dst = n.behind(dst)

	hops := defaultHops
	if raw := r.Header.Get(HeaderHops); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			hops = v
		}
	}

	dialCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	dialer := n.dialerFor(dialCtx, grant, route, hops, origin{})
	if n.stale(dst) && endsHere(dialer, dst.Addr()) {
		cancel()
		w.WriteHeader(http.StatusGone)
		return
	}
	s := n.counting(grant, r, route)

	if r.Header.Get(HeaderProto) == "icmp" {
		n.servePing(w, r, dialer, dst.Addr())
		cancel()
		return
	}

	if r.Header.Get(HeaderProto) == "udp" {
		if slot, err := strconv.ParseUint(r.Header.Get(HeaderBind), 10, 32); err == nil && s != nil && slot < markSlots {
			dialer = n.dialerFor(dialCtx, grant, route, hops, origin{s: s, slot: uint32(slot)})
		}
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
		out.Close()
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	out, err := dialer.DialTCP(dialCtx, dst)
	cancel()
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer out.Close()
	defer context.AfterFunc(r.Context(), func() { out.Close() })()

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
	return d.node.dialerFor(ctx, d.grant, d.route(ctx), d.hops, origin{}).DialTCP(ctx, dst)
}

func (d steered) DialUDP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	dst = d.node.behind(dst)
	from := origin{}
	if flow, ok := netstack.FlowOf(ctx); ok {
		from = origin{s: d.s, slot: slotOf(flow.Src.Addr().Is6() && !flow.Src.Addr().Is4In6(), flow.Src.Port())}
	}
	return d.node.dialerFor(ctx, d.grant, d.route(ctx), d.hops, from).DialUDP(ctx, dst)
}

func (d steered) route(ctx context.Context) string {
	flow, ok := netstack.FlowOf(ctx)
	if !ok || !d.s.marked {
		return d.s.heading()
	}
	if d.s.leaves(flow.Src) {
		return AnyExit
	}
	return ""
}

const flowQuiet = 2 * time.Minute

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

func settles(qc *quic.Conn) bool {
	if qc == nil {
		return true
	}
	select {
	case <-qc.HandshakeComplete():
	case <-qc.Context().Done():
	}
	return qc.Context().Err() == nil
}
