package qcli

import (
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

	"github.com/jaywehosl/quic-diver/internal/ippkt"
	"github.com/jaywehosl/quic-diver/internal/qcli/connectdial"
	"github.com/jaywehosl/quic-diver/internal/qcli/guard"
	"github.com/jaywehosl/quic-diver/internal/qcli/hybrid"
	"github.com/jaywehosl/quic-diver/internal/qcli/nat"
	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
	"github.com/jaywehosl/quic-diver/internal/qsrv"
	"github.com/jaywehosl/quic-diver/internal/qsrv/server/netstack"
	"github.com/jaywehosl/quic-diver/internal/qsrv/transport/cip"
	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink/quicconn"
	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink/relay"
	"github.com/jaywehosl/quic-diver/internal/roads"
)

type Options struct {
	Endpoints []string
	Relays    []relay.Link
	Token     string
	Device    string
	Route     string
	MTU       int
	Brutal    int
	Workers   int
	Resolver  string
	Exit      func(src, dst netip.AddrPort, udp bool) string
	Direct    func(pkt []byte) bool
	Bypass    []netip.Prefix
	Fast      func()
	Loud      bool
	Keep      func(fd uintptr)
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
		fmt.Printf("carriage cubic\n")
	}
	if len(opts.Endpoints) == 0 {
		return nil, fmt.Errorf("no entrypoint to dial")
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

	paths := len(opts.Endpoints)
	line := make(chan finish, paths+1)

	for _, endpoint := range opts.Endpoints {
		go func(where string) {
			t, err := reach(round, opts, where)
			if err != nil {
				line <- finish{err: fmt.Errorf("%s: %w", where, err)}
				return
			}
			if won(t) {
				line <- finish{tunnel: t}
			} else {
				line <- finish{err: fmt.Errorf("%s lost the race", where)}
			}
		}(endpoint)
	}

	if len(opts.Relays) > 0 {
		paths++
		go func() {
			fora := relayHeadStart
			if roads.RelayMode() {
				fora = 0
			}
			select {
			case <-time.After(fora):
			case <-round.Done():
				line <- finish{err: round.Err()}
				return
			}
			t := dialRelays(round, opts)
			if t == nil {
				line <- finish{err: fmt.Errorf("no relay answered")}
				return
			}
			if won(t) {
				line <- finish{tunnel: t}
			} else {
				line <- finish{err: fmt.Errorf("relay lost the race")}
			}
		}()
	}

	refused := []string{}
	for i := 0; i < paths; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case got := <-line:
			if got.err != nil {
				refused = append(refused, got.err.Error())
				continue
			}
			fmt.Printf("race     %s answered first in %d ms\n",
				got.tunnel.endpoint, time.Since(began).Milliseconds())
			roads.SetRelay(got.tunnel.relay != nil)
			return got.tunnel, nil
		}
	}

	if len(refused) == 0 {
		return nil, fmt.Errorf("no entrypoint answered")
	}
	return nil, fmt.Errorf("no entrypoint answered: %s", strings.Join(refused, "; "))
}

const relayHeadStart = 800 * time.Millisecond

func relayQUIC() *quic.Config {
	c := quicconn.DefaultConfig()
	c.HandshakeIdleTimeout = 15 * time.Second
	return c
}

func dialRelays(ctx context.Context, opts Options) *Tunnel {
	links := make([]relay.Link, 0, len(opts.Relays))
	for _, link := range opts.Relays {
		if link.Weblink != "" && link.Authority != "" {
			links = append(links, link)
		}
	}

	round, stop := context.WithCancel(ctx)
	defer stop()

	line := make(chan *Tunnel, len(links))
	for _, link := range links {
		go func(link relay.Link) {
			t, err := reachRelay(round, opts, link)
			if err != nil {
				fmt.Printf("relay    %s via %s failed: %v\n", link.Authority, link.Weblink, err)
			}
			line <- t
		}(link)
	}

	for i := range links {
		t := <-line
		if t == nil {
			continue
		}
		fmt.Printf("relay    %s up over cursor-relay via %s\n", t.endpoint, t.weblink)
		go func(left int) {
			for ; left > 0; left-- {
				if late := <-line; late != nil {
					late.Close()
				}
			}
		}(len(links) - i - 1)
		return t
	}
	return nil
}

func reachRelay(ctx context.Context, opts Options, link relay.Link) (*Tunnel, error) {
	host, _, err := net.SplitHostPort(link.Authority)
	if err != nil {
		return nil, err
	}

	sess := relay.New(relay.Config{Public: link.Weblink, Keep: opts.Keep})
	sess.Log = func(f string, a ...any) { fmt.Printf(f+"\n", a...) }
	qc, err := quicconn.OverRelay(ctx, sess, link.Authority, relayQUIC())
	if err != nil {
		return nil, err
	}

	tmpl := qsrv.Template(link.Authority, qsrv.ConnectIPPath)
	authURL := "https://" + link.Authority + qsrv.AuthPath
	client, err := cip.DialAuthConn(ctx, qc, tmpl, opts.Token, opts.Device, opts.Route, authURL)
	if err != nil {
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
		peers:    resolve(ctx, host),
		relay:    sess,
		weblink:  link.Weblink,
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

func reach(ctx context.Context, opts Options, endpoint string) (*Tunnel, error) {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, fmt.Errorf("endpoint %q: %w", endpoint, err)
	}

	tmpl := qsrv.Template(endpoint, qsrv.ConnectIPPath)
	tlsConf := &tls.Config{ServerName: host}
	authURL := "https://" + endpoint + qsrv.AuthPath

	round, stop := context.WithCancel(ctx)
	defer stop()

	type finish struct {
		road road
		err  error
	}
	line := make(chan finish, 2)
	paths := 2
	if roads.OnlyTCP() {
		paths = 1
	}

	if !roads.OnlyTCP() {
		go func() {
			client, err := cip.DialAuth(round, endpoint, tmpl, tlsConf,
				opts.Token, opts.Device, opts.Route, authURL, opts.Keep)
			if err != nil {
				line <- finish{err: fmt.Errorf("quic: %w", err)}
				return
			}
			line <- finish{road: client}
		}()
	}

	go func() {
		select {
		case <-time.After(roads.HeadStart(endpoint)):
		case <-round.Done():
			line <- finish{err: round.Err()}
			return
		}
		over, err := cip.DialOver(round, endpoint, opts.Token, opts.Device, opts.Route, authURL, opts.Keep)
		if err != nil {
			line <- finish{err: fmt.Errorf("tcp: %w", err)}
			return
		}
		line <- finish{road: over}
	}()

	var refused []string
	for i := 0; i < paths; i++ {
		got := <-line
		if got.err != nil {
			refused = append(refused, got.err.Error())
			continue
		}

		assigned, err := got.road.LocalPrefixes(ctx)
		if err != nil {
			got.road.Close()
			refused = append(refused, fmt.Sprintf("the node assigned no address: %v", err))
			continue
		}

		_, overTCP := got.road.(*cip.Over)
		roads.Remember(endpoint, overTCP)
		if overTCP {
			fmt.Printf("carriage %s over tcp, datagrams ride an h2 stream\n", endpoint)
		}

		t := &Tunnel{
			road:     got.road,
			overTCP:  overTCP,
			opts:     opts,
			endpoint: endpoint,
			assigned: assigned,
			peers:    resolve(ctx, host),
		}
		tag := opts.Route
		t.route.Store(&tag)

		left := paths - i - 1
		go func() {
			for k := 0; k < left; k++ {
				if late := <-line; late.road != nil {
					late.road.Close()
				}
			}
		}()
		return t, nil
	}
	return nil, errors.New(strings.Join(refused, " / "))
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

func (t *Tunnel) Path() roads.Path {
	return roads.Path{Endpoint: t.endpoint, OverTCP: t.overTCP, Relay: t.weblink}
}

func (t *Tunnel) Peers() []netip.Addr { return t.peers }

func (t *Tunnel) Close() error {
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
	for _, p := range t.assigned {
		assigned = append(assigned, p.Addr())
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
		NAT:      nat.New(assigned),
		Stack:    ns,
		Workers:  t.opts.Workers,
		Meter:    &t.meter,
		Fast:     t.opts.Fast,
		CatchDNS: t.opts.Resolver != "",
		Direct:   t.opts.Direct,
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

func (t *Tunnel) exitOf(f netstack.Flow) string {
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
	}
}

type routed struct {
	inner    connectdial.Dialer
	route    *atomic.Pointer[string]
	resolver string
	exit     func(src, dst netip.AddrPort, udp bool) string
	took     func(f netstack.Flow, tag string)
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
	return r.with(ctx).DialTCP(ctx, dst)
}

func (r routed) DialUDP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	if dst.Port() == 53 && r.resolver != "" {
		return net.Dial("udp", r.resolver)
	}
	return r.with(ctx).DialUDP(ctx, dst)
}

type Counters struct {
	Out, In, Back, BytesOut, BytesIn uint64
}

func (t *Tunnel) Stats() Counters {
	return Counters{
		Out:      t.meter.Out.Load(),
		In:       t.meter.In.Load(),
		Back:     t.meter.Back.Load(),
		BytesOut: t.meter.BytesOut.Load(),
		BytesIn:  t.meter.BytesIn.Load(),
	}
}

func (t *Tunnel) markOf(pkt []byte) uint64 {
	src, dst, udp, ok := ippkt.Flow(pkt)
	if exitTag(currentRoute(&t.route), t.opts.Exit, netstack.Flow{Src: src, Dst: dst, UDP: udp}, ok) == qsrv.AnyExit {
		return qsrv.MarkEgress
	}
	return qsrv.MarkHere
}

func (t *Tunnel) Endpoint() string { return t.endpoint }

func (t *Tunnel) CanMigrate() bool { return !t.overTCP && t.relay == nil }
