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

	"github.com/jaywehosl/quic-diver/internal/qcli"
	"github.com/jaywehosl/quic-diver/internal/qcli/guard"
	windivert "github.com/jaywehosl/quic-diver/internal/qcli/wdsource"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
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
	dns      *resolver

	assigned  netip.Prefix
	serverIP  string
	endpoint  string
	sessionID uint32
	since     time.Time
	lastErr   error
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

func (t *tunnel) Since() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.since
}

func (t *tunnel) MTU() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cfg.MTU
}

func (t *tunnel) SetMTU(mtu int) {
	t.mu.Lock()
	t.cfg.MTU = mtu
	t.mu.Unlock()
}

func (t *tunnel) ServerIP() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.serverIP
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

func (t *tunnel) DNS() *resolver {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.dns
}

func (t *tunnel) ReclaimDNS() {}

func (t *tunnel) servesDNS() bool { return t.cfg.DNS && t.cfg.Key != nil }

func (t *tunnel) token() string {
	if t.cfg.Token != nil {
		if token := t.cfg.Token(); token != "" {
			return token
		}
	}
	return hex.EncodeToString(t.cfg.Key[:])
}

func (t *tunnel) Start(servers []string, sessionID uint32) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return errAlreadyUp
	}
	if t.cfg.Key == nil {
		return fmt.Errorf("no network key yet")
	}
	if len(servers) == 0 {
		return fmt.Errorf("no entrypoint to dial")
	}

	dll, err := unpackDriver()
	if err != nil {
		return err
	}

	began := time.Now()
	ctx, cancel := context.WithCancel(context.Background())

	resolverAddr := ""
	var dns *resolver
	if t.servesDNS() {
		dns, err = newResolver("127.0.0.1:0", *t.cfg.Key, servers[0], t.token())
		if err != nil {
			cancel()
			return fmt.Errorf("dns: %w", err)
		}
		dns.OnQuery(t.cfg.OnQuery)
		resolverAddr = dns.Addr()
	}

	sharp := sharpTimers()

	keepOut := t.peerAddresses()

	dialCtx, dialStop := context.WithTimeout(ctx, 20*time.Second)
	live, err := qcli.Dial(dialCtx, qcli.Options{
		Endpoints: servers,
		Token:     t.token(),
		Device:    deviceOf().ID,
		Route:     routeTag(),
		MTU:       t.cfg.MTU,
		Brutal:    rateNow(),
		Workers:   t.cfg.Workers,
		Resolver:  resolverAddr,
		Bypass:    keepOut,
		Fast:      runFast,
		Exit:      exitFor,
		Direct:    goesDirect,
		Loud:      true,
	})
	dialStop()
	if err != nil {
		sharp()
		cancel()
		return err
	}

	assigned := live.Assigned()
	if len(assigned) == 0 {
		sharp()
		live.Close()
		cancel()
		return fmt.Errorf("the node assigned no address")
	}

	if dns != nil {
		dns.SetNode(live.Endpoint())
	}

	serverIP := ""
	for _, p := range live.Peers() {
		if p.Is4() {
			serverIP = p.String()
			break
		}
	}

	filter := windivert.BuildFilter(windivert.CaptureConfig{TCP: true, UDP: true, DNS: t.servesDNS(), Bypass: t.bypass(live, keepOut)})

	src, err := windivert.Open(dll, filter, 0)
	if err != nil {
		sharp()
		live.Close()
		cancel()
		return fmt.Errorf("windivert: %w (run as administrator)", err)
	}

	if r := routeByProcess.Load(); r != nil {
		go r.watchSockets(ctx, dll)
	}

	stop := make(chan struct{})

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		defer sharp()
		defer src.Close()
		err := live.Run(ctx, src)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = errors.New("the data path stopped")
		}
		fmt.Printf("carry    stopped: %v\n", err)
		go func() {
			t.Stop()
			t.noteResult(err)
			if t.cfg.Lost != nil {
				t.cfg.Lost()
			}
		}()
	}()

	t.running = true
	t.stop = stop
	t.live = live
	t.liveStop = cancel
	t.assigned = assigned[0]
	t.serverIP = serverIP
	t.endpoint = live.Endpoint()
	t.sessionID = sessionID
	t.since = time.Now()
	t.lastErr = nil
	t.dns = dns
	liveTunnel.Store(&live)

	go roamWatch(ctx, stop, live, func(err error) {
		fmt.Printf("carry    stopped: %v\n", err)
		go func() {
			t.Stop()
			t.noteResult(err)
			if t.cfg.Lost != nil {
				t.cfg.Lost()
			}
		}()
	})

	if dns != nil {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			dns.Serve(stop)
		}()
		go dns.keepWarm(stop)
		fmt.Printf("dns      answering on %s through the node\n", resolverAddr)
	}

	if t.cfg.Announce != nil {
		go t.cfg.Announce("join")
	}

	fmt.Printf("tunnel   up in %d ms, node gave %s\n", time.Since(began).Milliseconds(), t.assigned)
	return nil
}

func (t *tunnel) bypass(live *qcli.Tunnel, keepOut []netip.Prefix) []netip.Prefix {
	out := append([]netip.Prefix(nil), guard.New(nil).Bypasses()...)
	for _, p := range live.Peers() {
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
	if t.cfg.Announce != nil {
		go t.cfg.Announce("bye")
	}

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

	liveTunnel.Store(nil)
	close(stop)
	if cancel != nil {
		cancel()
	}
	if dns != nil {
		dns.Interrupt()
	}
	// Закрытие QUIC шлёт прощание и ждёт ядро: на мёртвой сети это может стоить
	// десятков секунд, а кнопка «Отключиться» столько ждать не должна.
	if live != nil {
		go live.Close()
	}

	// Ждём остановку, но не бесконечно: кнопка «Отключиться» не должна зависеть
	// ни от узла, ни от того, разгрёб ли очереди датапуть. Не уложились — отпускаем
	// интерфейс, остатки дойдут сами.
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

func (t *tunnel) Release() { t.Stop() }

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

// ServerName — точка входа, через которую туннель сейчас поднят.
func (t *tunnel) ServerName() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.endpoint
}

const stopWait = 3 * time.Second
