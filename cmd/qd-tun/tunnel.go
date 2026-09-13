//go:build windows

package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/jaywehosl/quic-diver/internal/clientdns"
	"github.com/jaywehosl/quic-diver/internal/clientrun"
	"github.com/jaywehosl/quic-diver/internal/qcli"
	"github.com/jaywehosl/quic-diver/internal/qcli/guard"
	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
	"github.com/jaywehosl/quic-diver/internal/qcli/windivert"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink/relay"
)

type tunnelConfig struct {
	MTU     int
	Workers int
	DNS     bool

	Token func() string

	OnQuery func(name string) (block bool)

	Peers func() []string

	Announce func(op string)

	Lost func()

	Key *qdcrypt.Key
}

type tunnel struct {
	cfg tunnelConfig

	mu      sync.Mutex
	running bool
	stop    chan struct{}
	wg      sync.WaitGroup

	live     *qcli.Tunnel
	liveStop context.CancelFunc
	dns      *clientdns.Resolver

	endpoint string
	since    time.Time
	lastErr  error
}

var errAlreadyUp = errors.New("tunnel is already up")

func newTunnel(cfg tunnelConfig) *tunnel {
	if cfg.Workers < 1 {
		cfg.Workers = 1
	}
	holdToken(cfg.Key)
	return &tunnel{cfg: cfg}
}

func (t *tunnel) Running() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

func (t *tunnel) Failed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastErr != nil
}

func (t *tunnel) noteResult(err error) {
	t.mu.Lock()
	t.lastErr = err
	t.mu.Unlock()
}

func (t *tunnel) DNS() *clientdns.Resolver {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.dns
}

func (t *tunnel) servesDNS() bool { return t.cfg.DNS && t.cfg.Key != nil }

func (t *tunnel) token() string {
	if t.cfg.Token != nil {
		if token := t.cfg.Token(); token != "" {
			return token
		}
	}
	return hex.EncodeToString(t.cfg.Key[:])
}

func (t *tunnel) Start(servers []string, relays []relay.Link, sessionID uint32) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return errAlreadyUp
	}
	if t.cfg.Key == nil {
		return fmt.Errorf("no network key yet")
	}

	nodeTalk.SetRelays(relays)

	dll, err := unpackDriver()
	if err != nil {
		return err
	}

	sharp := sharpTimers()
	keepOut := t.peerAddresses()

	var plan clientrun.Plan
	if t.servesDNS() {
		plan.DNS = &clientdns.Config{
			Node: servers[0], Token: t.token(), Ask: nodeTalk.Ask,
			Blocked: t.cfg.OnQuery,
		}
	}
	plan.Dial = qcli.Options{
		Endpoints: servers,
		Relays:    relays,
		Token:     t.token(),
		Device:    deviceOf().ID,
		Route:     routeTag(),
		MTU:       t.cfg.MTU,
		Brutal:    rateNow(),
		Workers:   t.cfg.Workers,
		Bypass:    keepOut,
		Fast:      runFast,
		Exit:      exitFor,
		Direct:    goesDirect,
		Loud:      true,
	}
	plan.Wait = dialWait
	plan.Say = func(format string, args ...any) { fmt.Printf(format+"\n", args...) }
	plan.Lost = func(err error) {
		t.Stop()
		t.noteResult(err)
		if t.cfg.Lost != nil {
			t.cfg.Lost()
		}
	}
	plan.Source = func(ctx context.Context, live *qcli.Tunnel) (packet.Source, error) {
		filter := windivert.BuildFilter(windivert.CaptureConfig{
			TCP: true, UDP: true, DNS: t.servesDNS(),
			Bypass: t.bypass(live, keepOut),
		})
		src, err := windivert.Open(dll, filter, 0)
		if err != nil {
			return nil, fmt.Errorf("windivert: %w (run as administrator)", err)
		}
		if r := routeByProcess.Load(); r != nil {
			go r.watchSockets(ctx, dll)
		}
		return src, nil
	}

	held, err := clientrun.Carry(context.Background(), plan)
	if err != nil {
		sharp()
		return err
	}

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		defer sharp()
		<-held.Gone
	}()

	t.running = true
	t.stop = held.Halt
	t.live = held.Live
	t.liveStop = held.Quit
	t.endpoint = held.Endpoint
	t.since = time.Now()
	t.lastErr = nil
	t.dns = held.DNS
	liveTunnel.Store(&held.Live)

	go roamWatch(held.Ctx, held.Halt, held.Live, plan.Lost)

	if t.cfg.Announce != nil {
		go t.cfg.Announce("join")
	}
	return nil
}

const dialWait = 20 * time.Second

func (t *tunnel) bypass(live *qcli.Tunnel, keepOut []netip.Prefix) []netip.Prefix {
	out := append([]netip.Prefix(nil), guard.New(nil).Bypasses()...)
	for _, p := range live.Peers() {
		out = append(out, netip.PrefixFrom(p, p.BitLen()))
	}
	for _, p := range live.RelayPeers() {
		out = append(out, netip.PrefixFrom(p, p.BitLen()))
	}
	return append(out, keepOut...)
}

func (t *tunnel) peerAddresses() []netip.Prefix {
	if t.cfg.Peers == nil {
		return nil
	}

	out := []netip.Prefix{}
	for _, host := range t.cfg.Peers() {
		for _, addr := range addressesOf(host) {
			out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
		}
	}
	return out
}

func addressesOf(host string) []netip.Addr {
	if addr, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{addr}
	}

	ctx, stop := context.WithTimeout(context.Background(), lookupWait)
	defer stop()

	found, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		fmt.Printf("bypass   could not resolve %s: %v\n", host, err)
		return nil
	}

	out := make([]netip.Addr, 0, len(found))
	for _, addr := range found {
		out = append(out, addr.Unmap())
	}
	return out
}

const lookupWait = 3 * time.Second

func (t *tunnel) Stop() error {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return nil
	}
	stop, live, cancel, dns := t.stop, t.live, t.liveStop, t.dns
	t.running = false
	t.live = nil
	t.liveStop = nil
	t.dns = nil
	t.lastErr = nil
	t.mu.Unlock()

	if t.cfg.Announce != nil {
		go t.cfg.Announce("bye")
	}

	liveTunnel.Store(nil)
	close(stop)
	if cancel != nil {
		cancel()
	}
	if dns != nil {
		dns.Interrupt()
	}
	if live != nil {
		go live.Close()
	}

	done := make(chan struct{})
	go func() {
		t.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(stopWait):
		fmt.Printf("tunnel   still winding down, letting go\n")
	}

	fmt.Printf("tunnel   down\n")
	return nil
}

func (t *tunnel) SetKey(key *qdcrypt.Key) {
	t.mu.Lock()
	t.cfg.Key = key
	t.mu.Unlock()
	holdToken(key)
}

func holdToken(key *qdcrypt.Key) {
	if key == nil {
		nodeTalk.SetToken("")
		return
	}
	nodeTalk.SetToken(hex.EncodeToString(key[:]))
}

func unpackDriver() (string, error) {
	dir, err := windivert.DefaultDir()
	if err != nil {
		return "", fmt.Errorf("driver folder: %w", err)
	}
	dll, err := windivert.Extract(dir)
	if err != nil {
		return "", fmt.Errorf("unpack windivert: %w", err)
	}
	return dll, nil
}

func (t *tunnel) ServerName() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.endpoint
}

const stopWait = 3 * time.Second
