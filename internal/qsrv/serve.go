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
	"time"

	connectip "github.com/quic-go/connect-ip-go"
	quic "github.com/quic-go/quic-go"

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

		peer := ""
		if addr, ok := r.Context().Value(remoteKey{}).(string); ok {
			peer = addr
		}
		route := routeOf(r)
		if route == "" {
			route = sessionOf(r.Context()).heading()
		}
		go n.carry(ctx, conn, sessionOf(r.Context()).quic(), grant, peer, settled(route))
	}
}

type remoteKey struct{}

func WithRemote(ctx context.Context, addr string) context.Context {
	return context.WithValue(ctx, remoteKey{}, addr)
}

func routeOf(r *http.Request) string { return r.Header.Get(HeaderRoute) }

func (n *Node) carry(ctx context.Context, conn *connectip.Conn, qc *quic.Conn, grant Grant, peer, route string) {
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

	now := time.Now().Unix()
	s := &live{grant: grant, address: address, peer: peer, conn: qc, since: now}
	s.lastSeen.Store(now)
	s.route.Store(&route)

	n.mu.Lock()
	if was := n.held[grant.Seat]; was != nil {
		n.pool.give(was.address)
	}
	n.held[grant.Seat] = s
	n.mu.Unlock()

	defer func() {
		n.mu.Lock()
		if n.held[grant.Seat] == s {
			delete(n.held, grant.Seat)
		}
		n.mu.Unlock()
		n.pool.give(address)
		conn.Close()
		n.links.forget(grant.Seat)
	}()

	tun := counted{conn: conn, s: s}

	stack, err := netstack.NewWithMTU(steered{node: n, grant: grant, s: s, hops: defaultHops}, n.Tunables().MTU)
	if err != nil {
		return
	}
	stack.OnFlow(s.holdFlow)
	defer stack.Reset(tun, exitDrain)

	stack.Run(ctx, tun)
}

const exitDrain = 200 * time.Millisecond

type counted struct {
	conn *connectip.Conn
	s    *live
}

func (c counted) ReadPacket(b []byte) (int, error) {
	n, mark, err := c.conn.ReadPacketMarked(b)
	if err == nil {
		c.s.up.Add(uint64(n))
		c.s.pktUp.Add(1)
		c.s.lastSeen.Store(time.Now().Unix())
		c.s.noteMark(b[:n], mark)
	}
	return n, err
}

func (c counted) WritePacket(b []byte) ([]byte, error) {
	icmp, err := c.conn.WritePacket(b)
	if err == nil {
		c.s.down.Add(uint64(len(b)))
		c.s.pktDown.Add(1)
	}
	return icmp, err
}

// dialerFor выбирает, чем выпускать флоу. Метка клиента исполняется буквально:
// сказано «через выход» — значит только через выход. Недоступен — флоу не
// состоится, приложение получит отказ. Тихо выпустить трафик здесь нельзя:
// клиент считал бы, что идёт через другую страну, а шёл бы отсюда.
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

	won, endpoint, err := n.raceExit(ctx, route, grant.Seat)
	if err != nil {
		return refusing{why: err}
	}

	n.transits.Add(1)
	return chained{cc: won, ls: n.links, endpoint: endpoint, seat: grant.Seat, hops: hops - 1}
}

// refusing отказывает во всех дозвонах: выход недоступен, а подменять его
// локальным нельзя.
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
		n.cfg.Log("flow      refused a CONNECT to %s: no seat", r.Host)
		w.WriteHeader(http.StatusProxyAuthRequired)
		return
	}

	route := routeOf(r)
	if route == "" {
		route = sessionOf(r.Context()).heading()
	}
	route = settled(route)

	dst, err := netip.ParseAddrPort(r.Host)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Просить у узла адрес, до которого он не доберётся, незачем: без ответа флоу
	// висит на дозвоне и держит поток, а за ним ждут остальные. Отказ мгновенный.
	if dst.Addr().Is6() && !dst.Addr().Is4In6() && !n.holdsV6() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	// Подставной адрес NAT46 разворачиваем в настоящий IPv6 до дозвона — и до
	// транзита, чтобы соседний узел получил уже честный адрес.
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

	if r.Header.Get(HeaderProto) == "udp" {
		out, err := dialer.DialUDP(dialCtx, dst)
		cancel()
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		n.relayPackets(w, r, out)
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

	n.mu.Lock()
	s := n.held[grant.Seat]
	n.mu.Unlock()
	if s == nil {
		s = n.peerSession(grant)
	}

	done := make(chan struct{})
	go func() {
		written, _ := io.Copy(out, r.Body)
		if s != nil {
			s.up.Add(uint64(written))
			s.lastSeen.Store(time.Now().Unix())
		}
		if cw, ok := out.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		}
		close(done)
	}()

	read, _ := io.Copy(flushWriter{w}, out)
	if s != nil {
		s.down.Add(uint64(read))
	}
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

// Подставной адрес разворачивается в настоящий здесь: дальше по пути (и на
// соседнем узле, если флоу транзитный) едет уже честный IPv6.
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

func (n *Node) relayPackets(w http.ResponseWriter, r *http.Request, out net.Conn) {
	defer out.Close()

	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
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
		}
	}()

	var head [2]byte
	buf := make([]byte, 65535)
	for {
		read, err := out.Read(buf)
		if err != nil {
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
	}
	<-done
}
