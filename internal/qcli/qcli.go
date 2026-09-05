package qcli

import (
	"context"
	"crypto/tls"
	"encoding/binary"
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

	"github.com/jaywehosl/quic-diver/internal/qcli/connectdial"
	"github.com/jaywehosl/quic-diver/internal/qcli/guard"
	"github.com/jaywehosl/quic-diver/internal/qcli/hybrid"
	"github.com/jaywehosl/quic-diver/internal/qcli/nat"
	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
	"github.com/jaywehosl/quic-diver/internal/qsrv"
	"github.com/jaywehosl/quic-diver/internal/qsrv/server/netstack"
	"github.com/jaywehosl/quic-diver/internal/qsrv/transport/cip"
)

type Options struct {
	Endpoints []string
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
	client   *cip.Client
	opts     Options
	endpoint string
	assigned []netip.Prefix
	peers    []netip.Addr

	meter hybrid.Meter

	route    atomic.Pointer[string]
	stackNow atomic.Pointer[netstack.Stack]
	taken    sync.Map
}

// Dial поднимает туннель гонкой: запросы уходят ко ВСЕМ точкам входа сразу,
// побеждает та, что первой отдала адрес, остальные закрываются немедленно —
// чтобы узел не держал соединение, по которому не пойдёт трафик.
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
	line := make(chan finish, len(opts.Endpoints))

	var once sync.Once
	began := time.Now()

	for _, endpoint := range opts.Endpoints {
		go func(where string) {
			t, err := reach(round, opts, where)
			if err != nil {
				line <- finish{err: fmt.Errorf("%s: %w", where, err)}
				return
			}
			taken := false
			once.Do(func() { taken = true })
			if !taken {
				t.Close()
				line <- finish{err: fmt.Errorf("%s lost the race", where)}
				return
			}
			line <- finish{tunnel: t}
		}(endpoint)
	}

	refused := []string{}
	for range opts.Endpoints {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case got := <-line:
			if got.err != nil {
				refused = append(refused, got.err.Error())
				continue
			}
			fmt.Printf("race     %s answered first of %d in %d ms\n",
				got.tunnel.endpoint, len(opts.Endpoints), time.Since(began).Milliseconds())
			return got.tunnel, nil
		}
	}

	if len(refused) == 0 {
		return nil, fmt.Errorf("no entrypoint answered")
	}
	return nil, fmt.Errorf("no entrypoint answered: %s", strings.Join(refused, "; "))
}

// reach — один забег: полный дозвон до готового туннеля с назначенным адресом.
func reach(ctx context.Context, opts Options, endpoint string) (*Tunnel, error) {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, fmt.Errorf("endpoint %q: %w", endpoint, err)
	}

	tmpl := qsrv.Template(endpoint, qsrv.ConnectIPPath)
	tlsConf := &tls.Config{ServerName: host}
	authURL := "https://" + endpoint + qsrv.AuthPath

	client, _, err := cip.DialAuth(ctx, endpoint, tmpl, tlsConf, opts.Token, opts.Device, opts.Route, authURL, opts.Keep)
	if err != nil {
		return nil, err
	}

	assigned, err := client.LocalPrefixes(ctx)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("the node assigned no address: %w", err)
	}

	t := &Tunnel{
		client:   client,
		opts:     opts,
		endpoint: endpoint,
		assigned: assigned,
		peers:    resolve(ctx, host),
	}
	t.SetRoute(opts.Route)
	return t, nil
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

func (t *Tunnel) Peers() []netip.Addr { return t.peers }

func (t *Tunnel) Close() error { return t.client.Close() }

// Alive — жива ли сессия туннеля.
func (t *Tunnel) Alive() bool { return t.client.Alive() }

// DatagramLimit — сколько байт полезной нагрузки помещается в датаграмму на
// этом пути. Спрашиваем у QUIC заведомо большой датаграммой: в сеть она не
// уходит, зато ошибка называет точный предел.
func (t *Tunnel) DatagramLimit() int { return t.client.DatagramLimit() }

// Ask спрашивает узел, жив ли путь и помнит ли он эту сессию. Маршрут посылается
// текущий, поэтому вопрос ничего не меняет.
func (t *Tunnel) Ask(ctx context.Context) error {
	tag := ""
	if held := t.route.Load(); held != nil {
		tag = *held
	}
	return t.client.Ask(ctx, tag)
}

func (t *Tunnel) Rebind(ctx context.Context) error {
	return t.client.Migrate(ctx, &net.UDPAddr{Port: 0})
}

func (t *Tunnel) SetRoute(tag string) {
	if held := t.route.Load(); held != nil && *held == tag {
		return
	}
	t.route.Store(&tag)

	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	err := t.client.Steer(ctx, tag)
	done()
	if err != nil {
		fmt.Printf("route    the node did not take the new exit: %v\n", err)
		return
	}
	fmt.Printf("route    exit is now %q\n", tag)
}

// Run несёт трафик, пока жив контекст. Источник пакетов даёт вызывающий: на
// Windows это WinDivert, на телефоне — дескриптор от VpnService. Движку разницы
// нет, оба отдают сырые IP-пакеты.
func (t *Tunnel) Run(ctx context.Context, src packet.Source) error {
	assigned := make([]netip.Addr, 0, len(t.assigned))
	for _, p := range t.assigned {
		assigned = append(assigned, p.Addr())
	}
	rewriter := nat.New(assigned)

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

	eng := hybrid.New(guard.New(keepOut), rewriter, ns, t.opts.Workers, &t.meter)
	eng.Fast(t.opts.Fast)
	eng.CatchDNS(t.opts.Resolver != "")
	eng.Direct(t.opts.Direct)
	eng.Mark(t.markOf)
	eng.Loud(t.opts.Loud)

	defer func() { eng.Stack().Reset(t.client, resetDrain) }()

	return eng.Run(ctx, src, t.client)
}

// Reroute обрывает флоу, чей выход изменился. Дорога выбирается один раз, при
// дозвоне: смена правила без этого действовала только на новые соединения, а
// открытые продолжали идти прежним выходом, пока приложение само их не закроет.
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

	t.taken.Range(func(key, _ any) bool {
		if _, held := alive[key.(flowMark)]; !held {
			t.taken.Delete(key)
		}
		return true
	})

	if shut > 0 {
		fmt.Printf("route    %d flows dropped so the new rule takes hold now\n", shut)
	}
	return shut
}

// exitOf — каким выходом флоу поехал бы сейчас. Ровно то же решение, что
// принимает дозвон, иначе сравнивать было бы не с чем.
func (t *Tunnel) exitOf(f netstack.Flow) string {
	tag := ""
	if held := t.route.Load(); held != nil {
		tag = *held
	}
	if t.opts.Exit != nil {
		tag = t.opts.Exit(f.Src, f.Dst, f.UDP)
	}
	if tag == "" {
		tag = qsrv.HereExit
	}
	return tag
}

func (t *Tunnel) tookFlow(f netstack.Flow, tag string) {
	t.taken.Store(flowMark{src: f.Src, dst: f.Dst, udp: f.UDP}, tag)
}

type flowMark struct {
	src, dst netip.AddrPort
	udp      bool
}

func (t *Tunnel) stack() (*netstack.Stack, error) {
	return netstack.NewWithMTU(t.dialer(), t.opts.MTU)
}

func (t *Tunnel) dialer() routed {
	return routed{
		inner:    connectdial.Dialer{CC: t.client.H3Conn()},
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
	out := r.inner
	tag := ""
	if held := r.route.Load(); held != nil {
		tag = *held
	}
	if r.exit != nil {
		if flow, ok := netstack.FlowOf(ctx); ok {
			tag = r.exit(flow.Src, flow.Dst, flow.UDP)
		}
	}
	if tag == "" {
		tag = qsrv.HereExit
	}
	if flow, ok := netstack.FlowOf(ctx); ok && r.took != nil {
		r.took(flow, tag)
	}
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

const resetDrain = 300 * time.Millisecond

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
	tag := ""
	if held := t.route.Load(); held != nil {
		tag = *held
	}
	if t.opts.Exit != nil {
		if src, dst, udp, ok := flowOf(pkt); ok {
			tag = t.opts.Exit(src, dst, udp)
		}
	}
	if tag == qsrv.AnyExit {
		return qsrv.MarkEgress
	}
	return qsrv.MarkHere
}

func flowOf(pkt []byte) (src, dst netip.AddrPort, udp bool, ok bool) {
	var proto byte
	var rest []byte
	var from, to netip.Addr
	switch {
	case len(pkt) >= 20 && pkt[0]>>4 == 4:
		head := int(pkt[0]&0x0f) * 4
		if head < 20 || len(pkt) < head+4 {
			return src, dst, false, false
		}
		proto, rest = pkt[9], pkt[head:]
		from = netip.AddrFrom4([4]byte(pkt[12:16]))
		to = netip.AddrFrom4([4]byte(pkt[16:20]))
	case len(pkt) >= 44 && pkt[0]>>4 == 6:
		proto, rest = pkt[6], pkt[40:]
		from = netip.AddrFrom16([16]byte(pkt[8:24]))
		to = netip.AddrFrom16([16]byte(pkt[24:40]))
	default:
		return src, dst, false, false
	}
	if proto != 6 && proto != 17 {
		return src, dst, false, false
	}
	return netip.AddrPortFrom(from, binary.BigEndian.Uint16(rest[0:2])),
		netip.AddrPortFrom(to, binary.BigEndian.Uint16(rest[2:4])), proto == 17, true
}

// Endpoint — точка входа, выигравшая гонку.
func (t *Tunnel) Endpoint() string { return t.endpoint }
