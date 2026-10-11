package qcli

import (
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	quic "github.com/quic-go/quic-go"

	"github.com/jaywehosl/qd/internal/ippkt"
	"github.com/jaywehosl/qd/internal/pace"
	"github.com/jaywehosl/qd/internal/qcli/connectdial"
	"github.com/jaywehosl/qd/internal/qcli/guard"
	"github.com/jaywehosl/qd/internal/qcli/hybrid"
	"github.com/jaywehosl/qd/internal/qcli/nat"
	"github.com/jaywehosl/qd/internal/qcli/packet"
	"github.com/jaywehosl/qd/internal/qsrv"
	"github.com/jaywehosl/qd/internal/qsrv/server/netstack"
	"github.com/jaywehosl/qd/internal/qsrv/transport/cip"
	"github.com/jaywehosl/qd/internal/qsrv/uplink/quicconn"
	"github.com/jaywehosl/qd/internal/qsrv/uplink/relay"
	"github.com/jaywehosl/qd/internal/roads"
	"github.com/jaywehosl/qd/internal/roots"
)

type Options struct {
	Endpoints []string
	Relays    []relay.Link
	Token     string
	Device    string
	Route     string
	MTU       int
	Brutal    int
	BBR       string
	Workers   int
	Resolver  string
	Exit      func(src, dst netip.AddrPort, udp bool) string
	Direct    func(pkt []byte) bool
	Detour    func(dst netip.Addr) bool
	Outside   func(ctx context.Context, network string, dst netip.AddrPort) (net.Conn, error)
	Bypass    []netip.Prefix
	Fast      func()
	Loud      bool
	Keep      func(fd uintptr)
	Tickets   tls.ClientSessionCache
}

type Tunnel struct {
	road     road
	overTCP  bool
	opts     Options
	endpoint string
	assigned []netip.Prefix
	peers    []netip.Addr
	relay    *relay.Session
	weblink  string

	meter hybrid.Meter

	route    atomic.Pointer[string]
	stackNow atomic.Pointer[netstack.Stack]
	taken    sync.Map
	marks    atomic.Uint64
	better   chan struct{}
	gone     chan struct{}
	hurry    chan struct{}
	once     sync.Once
	left     sync.Once
}

func Dial(ctx context.Context, opts Options) (*Tunnel, error) {
	if opts.MTU <= 0 {
		opts.MTU = 1500
	}
	if opts.Workers <= 0 {
		opts.Workers = 1
	}
	if opts.Brutal > 0 {
		os.Setenv("QD_BRUTAL_MBPS", strconv.Itoa(opts.Brutal))
		fmt.Printf("carriage brutal, %d Mbit/s regardless of loss\n", opts.Brutal)
	} else {
		os.Unsetenv("QD_BRUTAL_MBPS")
		os.Setenv("QD_BBR_PROFILE", opts.BBR)
		fmt.Printf("carriage bbr, %s profile\n", cmp.Or(opts.BBR, "standard"))
	}
	if len(opts.Endpoints) == 0 {
		return nil, fmt.Errorf("no entrypoint to dial")
	}

	straight := map[string]bool{}
	links := map[string][]relay.Link{}
	var lanes []string
	for _, endpoint := range opts.Endpoints {
		if !straight[endpoint] {
			straight[endpoint] = true
			lanes = append(lanes, endpoint)
		}
	}
	for _, link := range opts.Relays {
		if link.Weblink == "" || link.Authority == "" {
			continue
		}
		if !straight[link.Authority] && links[link.Authority] == nil {
			lanes = append(lanes, link.Authority)
		}
		links[link.Authority] = append(links[link.Authority], link)
	}

	round, stop := context.WithCancel(ctx)
	defer stop()

	type finish struct {
		tunnel *Tunnel
		err    error
	}

	var once sync.Once
	began := time.Now()
	won := func(t *Tunnel) bool {
		taken := false
		once.Do(func() { taken = true })
		if !taken {
			t.Close()
		}
		return taken
	}

	line := make(chan finish, len(lanes))
	for _, where := range lanes {
		go func(where string) {
			t, err := climb(round, opts, where, straight[where], links[where])
			if err != nil {
				line <- finish{err: fmt.Errorf("%s: %w", where, err)}
				return
			}
			if won(t) {
				line <- finish{tunnel: t}
			} else {
				line <- finish{err: fmt.Errorf("%s lost the race", where)}
			}
		}(where)
	}

	refused := []string{}
	for i := range lanes {
		select {
		case <-ctx.Done():
			go func(left int) {
				for ; left > 0; left-- {
					if late := <-line; late.tunnel != nil {
						late.tunnel.Close()
					}
				}
			}(len(lanes) - i)
			return nil, ctx.Err()
		case got := <-line:
			if got.err != nil {
				refused = append(refused, got.err.Error())
				continue
			}
			fmt.Printf("race     %s answered first in %d ms\n",
				got.tunnel.endpoint, time.Since(began).Milliseconds())
			return got.tunnel, nil
		}
	}

	if len(refused) == 0 {
		return nil, fmt.Errorf("no entrypoint answered")
	}
	return nil, fmt.Errorf("no entrypoint answered: %s", strings.Join(refused, "; "))
}

const slowAddress = 2 * time.Second

func climb(ctx context.Context, opts Options, endpoint string, straight bool, links []relay.Link) (*Tunnel, error) {
	var tries []roads.Try[*Tunnel]
	if straight && !roads.OnlyTCP() {
		tries = append(tries, roads.Try[*Tunnel]{Rung: roads.QUIC, Run: func(ctx context.Context) (*Tunnel, error) {
			return overQUIC(ctx, opts, endpoint)
		}})
	}
	if straight {
		tries = append(tries, roads.Try[*Tunnel]{Rung: roads.TCP, Run: func(ctx context.Context) (*Tunnel, error) {
			return overTCP(ctx, opts, endpoint)
		}})
	}
	for _, link := range links {
		tries = append(tries, roads.Try[*Tunnel]{Rung: roads.Relay, Run: func(ctx context.Context) (*Tunnel, error) {
			t, err := reachRelay(ctx, opts, link)
			if err != nil && ctx.Err() == nil {
				fmt.Printf("relay    %s via %s failed: %v\n", link.Authority, link.Weblink, err)
			}
			return t, err
		}})
	}

	var held atomic.Pointer[Tunnel]
	var climbed atomic.Bool
	t, rung, err := roads.Climb(ctx, endpoint, tries, func(late *Tunnel) { late.Close() }, func() {
		climbed.Store(true)
		if up := held.Load(); up != nil {
			up.improve()
		}
	})
	if err != nil {
		return nil, err
	}
	held.Store(t)
	if climbed.Load() {
		t.improve()
	}

	switch rung {
	case roads.TCP:
		fmt.Printf("carriage %s over tcp, datagrams ride an h2 stream\n", endpoint)
		if over, ok := t.road.(*cip.Over); ok && !over.Marked() {
			fmt.Printf("carriage this node takes no exit per packet on tcp, udp of every app follows the global exit\n")
		}
	case roads.Relay:
		fmt.Printf("relay    %s up over cursor-relay via %s\n", t.endpoint, t.weblink)
	}
	if rung != roads.QUIC {
		go t.seek(rung)
	}
	return t, nil
}

func overQUIC(ctx context.Context, opts Options, endpoint string) (*Tunnel, error) {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, fmt.Errorf("endpoint %q: %w", endpoint, err)
	}
	tlsConf := &tls.Config{ServerName: host, ClientSessionCache: opts.Tickets, RootCAs: roots.Pool()}
	client, err := cip.DialAuth(ctx, endpoint, qsrv.Template(endpoint, qsrv.ConnectIPPath), tlsConf,
		opts.Token, opts.Device, opts.Route, "https://"+endpoint+qsrv.AuthPath, opts.Keep)
	if err != nil {
		return nil, fmt.Errorf("quic: %w", err)
	}
	return settle(ctx, opts, endpoint, client, false)
}

func overTCP(ctx context.Context, opts Options, endpoint string) (*Tunnel, error) {
	over, err := cip.DialOver(ctx, endpoint, opts.Token, opts.Device, opts.Route, "https://"+endpoint+qsrv.AuthPath, opts.Keep)
	if err != nil {
		return nil, fmt.Errorf("tcp: %w", err)
	}
	return settle(ctx, opts, endpoint, over, true)
}

func settle(ctx context.Context, opts Options, endpoint string, held road, overTCP bool) (*Tunnel, error) {
	began := time.Now()
	assigned, err := held.LocalPrefixes(ctx)
	if took := time.Since(began); took > slowAddress {
		fmt.Printf("carry    the node gave an address %d ms after the road was up\n", took.Milliseconds())
	}
	if err != nil {
		held.Close()
		return nil, fmt.Errorf("the node assigned no address: %w", err)
	}
	t := &Tunnel{
		road:     held,
		overTCP:  overTCP,
		opts:     opts,
		endpoint: endpoint,
		assigned: assigned,
		peers:    peersOf(ctx, endpoint),
		better:   make(chan struct{}),
		gone:     make(chan struct{}),
		hurry:    make(chan struct{}, 1),
	}
	tag := opts.Route
	t.route.Store(&tag)
	return t, nil
}

func relayQUIC() *quic.Config {
	c := quicconn.DefaultConfig()
	c.HandshakeIdleTimeout = 15 * time.Second
	return c
}

func reachRelay(ctx context.Context, opts Options, link relay.Link) (*Tunnel, error) {
	host, _, err := net.SplitHostPort(link.Authority)
	if err != nil {
		return nil, err
	}

	sess := relay.New(relay.Config{Public: link.Weblink, Keep: opts.Keep})
	sess.Log = func(f string, a ...any) { fmt.Printf(f+"\n", a...) }
	qc, err := quicconn.OverRelay(ctx, sess, link.Authority, relayQUIC(), opts.Tickets)
	if err != nil {
		return nil, err
	}

	tmpl := qsrv.Template(link.Authority, qsrv.ConnectIPPath)
	authURL := "https://" + link.Authority + qsrv.AuthPath
	client, err := cip.DialAuthConn(ctx, qc, tmpl, opts.Token, opts.Device, opts.Route, authURL)
	if err != nil {
		if cip.Rejected0RTT(err) && opts.Tickets != nil {
			opts.Tickets.Put(host, nil)
		}
		sess.Stop()
		return nil, err
	}

	assigned, err := client.LocalPrefixes(ctx)
	if err != nil {
		client.Close()
		sess.Stop()
		return nil, err
	}

	t := &Tunnel{
		road:     client,
		opts:     opts,
		endpoint: link.Authority,
		assigned: assigned,
		peers:    peersOf(ctx, link.Authority),
		relay:    sess,
		weblink:  link.Weblink,
		better:   make(chan struct{}),
		gone:     make(chan struct{}),
		hurry:    make(chan struct{}, 1),
	}
	tag := opts.Route
	t.route.Store(&tag)
	return t, nil
}

func (t *Tunnel) RelayPeers() []netip.Addr {
	if t.relay == nil {
		return nil
	}
	return t.relay.Peers()
}

type road interface {
	Alive() bool
	Close() error
	Steer(ctx context.Context, route string) error
	Ask(ctx context.Context, route string) error
	LocalPrefixes(ctx context.Context) ([]netip.Prefix, error)
	DatagramLimit() int
	Migrate(ctx context.Context, laddr *net.UDPAddr) error
	ReadPacket(b []byte) (int, error)
	WritePacket(b []byte) (icmp []byte, err error)
}

func resolve(ctx context.Context, host string) []netip.Addr {
	if addr, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{addr}
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil
	}
	out := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.Unmap())
	}
	return out
}

func (t *Tunnel) Assigned() []netip.Prefix { return t.assigned }

func (t *Tunnel) Six() (netip.Prefix, bool) {
	for _, p := range t.assigned {
		if p.Addr().Is6() {
			return p, true
		}
	}
	return netip.Prefix{}, false
}

func (t *Tunnel) Path() roads.Path {
	hidden, _ := t.road.(interface{ Hidden() bool })
	return roads.Path{Endpoint: t.endpoint, OverTCP: t.overTCP, Relay: t.weblink, Hidden: hidden != nil && hidden.Hidden()}
}

func (t *Tunnel) Peers() []netip.Addr { return t.peers }

func (t *Tunnel) Close() error {
	if t.gone != nil {
		t.left.Do(func() { close(t.gone) })
	}
	err := t.road.Close()
	if t.relay != nil {
		t.relay.Stop()
	}
	return err
}

func (t *Tunnel) StopRelay() {
	if t.relay != nil {
		t.relay.Stop()
	}
}

func (t *Tunnel) Alive() bool { return t.road.Alive() }

func (t *Tunnel) DatagramLimit() int { return t.road.DatagramLimit() }

func (t *Tunnel) Ask(ctx context.Context) error {
	return t.road.Ask(ctx, currentRoute(&t.route))
}

func currentRoute(route *atomic.Pointer[string]) string {
	if held := route.Load(); held != nil {
		return *held
	}
	return ""
}

func exitTag(route string, exit func(src, dst netip.AddrPort, udp bool) string, flow netstack.Flow, known bool) string {
	if exit != nil && known {
		route = exit(flow.Src, flow.Dst, flow.UDP)
	}
	if route == "" {
		return qsrv.HereExit
	}
	return route
}

func (t *Tunnel) Rebind(ctx context.Context) error {
	return t.road.Migrate(ctx, &net.UDPAddr{Port: 0})
}

func (t *Tunnel) SetRoute(tag string) {
	if held := t.route.Load(); held != nil && *held == tag {
		return
	}
	t.route.Store(&tag)

	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	err := t.road.Steer(ctx, tag)
	done()
	if err != nil {
		fmt.Printf("route    the node did not take the new exit: %v\n", err)
		return
	}
	fmt.Printf("route    exit is now %q\n", tag)
}

func (t *Tunnel) Run(ctx context.Context, src packet.Source) error {
	assigned := make([]netip.Addr, 0, len(t.assigned))
	var gateway, gateway6 netip.Addr
	for _, p := range t.assigned {
		assigned = append(assigned, p.Addr())
		switch next := p.Masked().Addr().Next(); {
		case p.Addr().Is4() && !gateway.IsValid():
			gateway = next
		case p.Addr().Is6() && !gateway6.IsValid():
			gateway6 = next
		}
	}
	ns, err := t.stack()
	if err != nil {
		return err
	}
	t.stackNow.Store(ns)
	defer t.stackNow.Store(nil)

	keepOut := append([]netip.Addr{}, t.peers...)
	for _, p := range t.opts.Bypass {
		if p.IsSingleIP() {
			keepOut = append(keepOut, p.Addr())
		}
	}

	return hybrid.New(hybrid.Options{
		Guard:    guard.New(keepOut),
		Gateway:  gateway,
		Gateway6: gateway6,
		NAT:      nat.New(assigned),
		Stack:    ns,
		Workers:  t.opts.Workers,
		Meter:    &t.meter,
		Fast:     t.opts.Fast,
		CatchDNS: t.opts.Resolver != "",
		Direct:   t.opts.Direct,
		Detour:   t.detour(),
		Mark:     t.markOf,
		Loud:     t.opts.Loud,
	}).Run(ctx, src, t.road)
}

func (t *Tunnel) Reroute() int {
	ns := t.stackNow.Load()
	if ns == nil {
		return 0
	}

	alive := map[flowMark]struct{}{}
	shut := ns.ShutFlows(func(f netstack.Flow) bool {
		mark := flowMark{src: f.Src, dst: f.Dst, udp: f.UDP}
		alive[mark] = struct{}{}
		went, known := t.taken.Load(mark)
		if !known {
			return false
		}
		return t.exitOf(f) != went.(string)
	})
	t.keepOnly(alive)

	if shut > 0 {
		fmt.Printf("route    %d flows dropped so the new rule takes hold now\n", shut)
	}
	return shut
}

func (t *Tunnel) keepOnly(alive map[flowMark]struct{}) {
	t.taken.Range(func(key, _ any) bool {
		if _, held := alive[key.(flowMark)]; !held {
			t.taken.Delete(key)
		}
		return true
	})
}

func (t *Tunnel) forgetDeadFlows() {
	ns := t.stackNow.Load()
	if ns == nil {
		return
	}
	alive := map[flowMark]struct{}{}
	ns.ShutFlows(func(f netstack.Flow) bool {
		alive[flowMark{src: f.Src, dst: f.Dst, udp: f.UDP}] = struct{}{}
		return false
	})
	t.keepOnly(alive)
}

func (t *Tunnel) FlowDsts() []netip.Addr {
	ns := t.stackNow.Load()
	if ns == nil {
		return nil
	}
	var out []netip.Addr
	ns.ShutFlows(func(f netstack.Flow) bool {
		out = append(out, f.Dst.Addr())
		return false
	})
	return out
}

func (t *Tunnel) exitOf(f netstack.Flow) string {
	if t.opts.Detour != nil && t.opts.Detour(f.Dst.Addr()) {
		return asideTag
	}
	return exitTag(currentRoute(&t.route), t.opts.Exit, f, true)
}

func (t *Tunnel) tookFlow(f netstack.Flow, tag string) {
	t.taken.Store(flowMark{src: f.Src, dst: f.Dst, udp: f.UDP}, tag)
	if t.marks.Add(1)%flowSweep == 0 {
		go t.forgetDeadFlows()
	}
}

const flowSweep = 512

type flowMark struct {
	src, dst netip.AddrPort
	udp      bool
}

func (t *Tunnel) stack() (*netstack.Stack, error) {
	return netstack.New(t.dialer(), t.opts.MTU)
}

func (t *Tunnel) dialer() routed {
	inner := connectdial.Dialer{}
	switch held := t.road.(type) {
	case *cip.Client:
		inner.CC = held.H3Conn()
	case *cip.Over:
		inner.H2 = held.H2Conn()
	}
	return routed{
		inner:    inner,
		route:    &t.route,
		resolver: t.opts.Resolver,
		exit:     t.opts.Exit,
		took:     t.tookFlow,
		detour:   t.detour(),
		outside:  t.opts.Outside,
	}
}

const asideTag = "aside"

func (t *Tunnel) detour() func(dst netip.Addr) bool {
	if t.opts.Outside == nil {
		return nil
	}
	return t.opts.Detour
}

type routed struct {
	inner    connectdial.Dialer
	route    *atomic.Pointer[string]
	resolver string
	exit     func(src, dst netip.AddrPort, udp bool) string
	took     func(f netstack.Flow, tag string)
	detour   func(dst netip.Addr) bool
	outside  func(ctx context.Context, network string, dst netip.AddrPort) (net.Conn, error)
}

func (r routed) aside(ctx context.Context, network string, dst netip.AddrPort) (net.Conn, bool, error) {
	if r.detour == nil || !r.detour(dst.Addr()) {
		return nil, false, nil
	}
	if ippkt.StandIn.Contains(dst.Addr().Unmap()) {
		return nil, true, errors.New("a stand-in address means nothing outside the tunnel")
	}
	if flow, known := netstack.FlowOf(ctx); known {
		r.took(flow, asideTag)
	}
	conn, err := r.outside(ctx, network, dst)
	return conn, true, err
}

func (r routed) with(ctx context.Context) connectdial.Dialer {
	flow, known := netstack.FlowOf(ctx)
	tag := exitTag(currentRoute(r.route), r.exit, flow, known)
	if known {
		r.took(flow, tag)
	}
	out := r.inner
	out.Header = http.Header{qsrv.HeaderRoute: []string{tag}}
	return out
}

func (r routed) DialTCP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	if dst.Port() == 53 && r.resolver != "" {
		return dnsOverTCP(r.resolver)
	}
	if conn, aside, err := r.aside(ctx, "tcp", dst); aside {
		return conn, err
	}
	return r.with(ctx).DialTCP(ctx, dst)
}

func (r routed) DialUDP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	if dst.Port() == 53 && r.resolver != "" {
		return net.Dial("udp", r.resolver)
	}
	if conn, aside, err := r.aside(ctx, "udp", dst); aside {
		return conn, err
	}
	return r.with(ctx).DialUDP(ctx, dst)
}

type Counters struct {
	Out, In, Back, BytesOut, BytesIn uint64
	Heard                            uint64
}

func (t *Tunnel) Stats() Counters {
	out := Counters{
		Out:      t.meter.Out.Load(),
		In:       t.meter.In.Load(),
		Back:     t.meter.Back.Load(),
		BytesOut: t.meter.BytesOut.Load(),
		BytesIn:  t.meter.BytesIn.Load(),
	}
	out.Heard = out.In
	if held, ok := t.road.(interface{ Received() uint64 }); ok {
		out.Heard = held.Received()
	}
	return out
}

func (t *Tunnel) markOf(pkt []byte) uint64 {
	src, dst, udp, ok := ippkt.Flow(pkt)
	if exitTag(currentRoute(&t.route), t.opts.Exit, netstack.Flow{Src: src, Dst: dst, UDP: udp}, ok) == qsrv.AnyExit {
		return qsrv.MarkEgress
	}
	return qsrv.MarkHere
}

func (t *Tunnel) Endpoint() string { return t.endpoint }

type Carrier interface {
	RoundTrip(*http.Request) (*http.Response, error)
}

func (t *Tunnel) Carrier() Carrier {
	switch held := t.road.(type) {
	case *cip.Client:
		return held.H3Conn()
	case *cip.Over:
		return held.H2Conn()
	}
	return nil
}

func (t *Tunnel) Sign(h http.Header) {
	h.Set(qsrv.HeaderToken, t.opts.Token)
	if t.opts.Device != "" {
		h.Set(qsrv.HeaderDevice, t.opts.Device)
	}
}

func (t *Tunnel) Better() <-chan struct{} { return t.better }

func (t *Tunnel) OverTCP() bool { return t.overTCP }

func (t *Tunnel) Reseek() {
	select {
	case t.hurry <- struct{}{}:
	default:
	}
}

func (t *Tunnel) improve() { t.once.Do(func() { close(t.better) }) }

func (t *Tunnel) seek(rung roads.Rung) {
	host, _, err := net.SplitHostPort(t.endpoint)
	if err != nil {
		return
	}
	tick := time.NewTicker(pace.ProbeEvery)
	defer tick.Stop()
	for {
		select {
		case <-t.gone:
			return
		case <-tick.C:
		case <-t.hurry:
		}
		if !roads.OnlyTCP() {
			round, stop := context.WithTimeout(context.Background(), pace.ProbeWait)
			conn, err := quicconn.Dialer{
				TLS:  &tls.Config{ServerName: host, NextProtos: []string{"h3"}, RootCAs: roots.Pool()},
				Keep: t.opts.Keep,
			}.Dial(round, t.endpoint)
			stop()
			if err == nil {
				conn.Close()
				roads.Remember(t.endpoint, roads.QUIC)
				t.improve()
				return
			}
		}
		if rung != roads.Relay {
			continue
		}
		round, stop := context.WithTimeout(context.Background(), pace.ProbeWait)
		conn, _, err := cip.ReachH2(round, t.endpoint, t.opts.Keep)
		stop()
		if err != nil {
			continue
		}
		conn.Close()
		roads.Remember(t.endpoint, roads.TCP)
		t.improve()
		return
	}
}

func (t *Tunnel) CanMigrate() bool { return !t.overTCP && t.relay == nil }

func (t *Tunnel) Reaches(ctx context.Context) bool {
	held, ok := t.road.(interface{ Reaches(context.Context) error })
	return !ok || held.Reaches(ctx) == nil
}

func peersOf(ctx context.Context, endpoint string) []netip.Addr {
	if held := quicconn.Known(endpoint); len(held) > 0 {
		return held
	}
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil
	}
	look, stop := context.WithTimeout(ctx, peerLookup)
	defer stop()
	return resolve(look, host)
}

const peerLookup = 4 * time.Second
