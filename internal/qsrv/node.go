package qsrv

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	connectip "github.com/quic-go/connect-ip-go"
	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/yosida95/uritemplate/v3"

	"github.com/jaywehosl/qd/internal/ippkt"
	"github.com/jaywehosl/qd/internal/qsrv/server/decoy"
	"github.com/jaywehosl/qd/internal/update"
)

const (
	AuthPath      = "/qd/hello"
	ConnectIPPath = "/qd/ip"
	IPOverTCPPath = "/qd/ipt"
	RPCPath       = "/qd/rpc/"
	UpdatePath    = update.Path

	defaultHops = 2
)

type Grant struct {
	Client    string
	AllowExit bool
	Session   uint32
	Seat      uint32
	Via       uint32
	Peer      bool
}

type Tunables struct {
	MaxStreams   int64
	StreamWindow uint64
	MaxStreamWin uint64
	ConnWindow   uint64
	MaxConnWin   uint64
	IdleTimeout  time.Duration
	KeepAlive    time.Duration
	SocketBuffer int
	MTU          int
}

func DefaultTunables() Tunables {
	return Tunables{
		MaxStreams:   65536,
		StreamWindow: 512 << 10,
		MaxStreamWin: 16 << 20,
		ConnWindow:   96 << 20,
		MaxConnWin:   96 << 20,
		IdleTimeout:  90 * time.Second,
		KeepAlive:    15 * time.Second,
		SocketBuffer: 2 << 20,
		MTU:          1400,
	}
}

type Config struct {
	Listen    string
	Authority string
	SelfID    string
	SelfTag   string
	NATSlot   int
	Pool      netip.Prefix

	TLS   *tls.Config
	Token string

	Verify func(raw string) (Grant, bool)
	Admit  func(g Grant, version, build string) bool
	Seat   func(g Grant, device string) bool
	Reset  *quic.StatelessResetKey
	Shelf  *update.Shelf
	Peers  func() []Peer
	Tune   func() Tunables

	Ask func(op string, body []byte, auth string) (any, error)
	Log func(format string, args ...any)
}

type Session struct {
	Seat      uint32
	Session   uint32
	Client    string
	Address   netip.Prefix
	Addresses []netip.Prefix
	Peer      string
	Since     int64
	LastSeen  int64
	Up        uint64
	Down      uint64
	PktUp     uint64
	PktDown   uint64
	AllowExit bool
	Transit   bool
	Carriage  string
}

type live struct {
	grant   Grant
	address netip.Prefix
	peer    string
	conn    *quic.Conn
	since   int64
	transit bool
	tot     *totals
	stream  bool
	marked  bool
	end     func()
	over    atomic.Bool
	under   *live

	route  atomic.Pointer[string]
	flowMu sync.Mutex
	heads  map[flowAt]uint64
	flows  map[flowAt][]io.Closer

	outMu   sync.Mutex
	outlets map[uint32]*outlet

	up, down       atomic.Uint64
	pktUp, pktDown atomic.Uint64
	lastSeen       atomic.Int64
}

type Node struct {
	cfg   Config
	tune  atomic.Pointer[Tunables]
	pool  *pool
	nat   *nat46
	links *links

	exitRest atomic.Int64

	proxy *connectip.Proxy
	tmpl  *uritemplate.Template
	site  http.Handler

	transits atomic.Uint64
	refused  atomic.Uint64

	mu   sync.Mutex
	held map[uint32]*live

	tally sync.Map

	srv *http3.Server
}

type totalKey struct {
	session uint32
	transit bool
}

type totals struct {
	up, down, pktUp, pktDown atomic.Uint64
	lastSeen                 atomic.Int64
}

type Total struct {
	Session  uint32
	Transit  bool
	Up       uint64
	Down     uint64
	PktUp    uint64
	PktDown  uint64
	LastSeen int64
}

const tallyKeep = time.Hour

func (n *Node) total(session uint32, transit bool) *totals {
	key := totalKey{session, transit}
	if held, ok := n.tally.Load(key); ok {
		return held.(*totals)
	}
	fresh := &totals{}
	fresh.lastSeen.Store(time.Now().Unix())
	held, _ := n.tally.LoadOrStore(key, fresh)
	return held.(*totals)
}

func (n *Node) newLive(grant Grant, address netip.Prefix, peer, route string, transit bool) *live {
	now := time.Now().Unix()
	s := &live{grant: grant, address: address, peer: peer, since: now, transit: transit}
	s.tot = n.total(grant.Session, transit)
	s.lastSeen.Store(now)
	s.route.Store(&route)
	return s
}

func (n *Node) Totals() []Total {
	n.mu.Lock()
	inUse := make(map[totalKey]bool, len(n.held))
	for _, s := range n.held {
		inUse[totalKey{s.grant.Session, s.transit}] = true
	}
	n.mu.Unlock()

	cut := time.Now().Add(-tallyKeep).Unix()
	out := []Total{}
	n.tally.Range(func(k, v any) bool {
		key, t := k.(totalKey), v.(*totals)
		if !inUse[key] && t.lastSeen.Load() < cut {
			n.tally.Delete(k)
			return true
		}
		out = append(out, Total{
			Session: key.session, Transit: key.transit,
			Up: t.up.Load(), Down: t.down.Load(), PktUp: t.pktUp.Load(), PktDown: t.pktDown.Load(),
			LastSeen: t.lastSeen.Load(),
		})
		return true
	})
	return out
}

func New(cfg Config) *Node {
	if cfg.Log == nil {
		cfg.Log = func(string, ...any) {}
	}
	if !cfg.Pool.IsValid() {
		cfg.Pool = netip.MustParsePrefix("10.7.0.0/16")
	}

	n := &Node{
		cfg:   cfg,
		pool:  newPool(cfg.Pool),
		nat:   newNAT46(cfg.NATSlot),
		links: newLinks(cfg.Token, cfg.SelfID, cfg.Log),
		proxy: &connectip.Proxy{},
		tmpl:  Template(cfg.Authority, ConnectIPPath),
		site:  decoy.Handler(),
		held:  map[uint32]*live{},
	}
	t := DefaultTunables()
	if cfg.Tune != nil {
		t = cfg.Tune()
	}
	n.Retune(t)
	return n
}

func Template(authority, path string) *uritemplate.Template {
	return uritemplate.MustNew(fmt.Sprintf("https://%s%s", authority, path))
}

func (n *Node) Retune(t Tunables) {
	n.tune.Store(&t)
}

func (n *Node) Tunables() Tunables { return *n.tune.Load() }

func (n *Node) Live() (sessions int, transits, refused uint64) {
	n.mu.Lock()
	sessions = len(n.held)
	n.mu.Unlock()
	return sessions, n.transits.Load(), n.refused.Load()
}

func (n *Node) Sessions() []Session {
	n.mu.Lock()
	defer n.mu.Unlock()

	out := make([]Session, 0, len(n.held))
	for seat, s := range n.held {
		id := s.grant.Session
		if id == 0 {
			id = seat
		}
		out = append(out, Session{
			Session:   id,
			Seat:      seat,
			Client:    s.grant.Client,
			Address:   s.address,
			Addresses: carried(s.address),
			Peer:      s.where(),
			Since:     s.since,
			LastSeen:  s.lastSeen.Load(),
			Up:        s.up.Load(),
			Down:      s.down.Load(),
			PktUp:     s.pktUp.Load(),
			PktDown:   s.pktDown.Load(),
			AllowExit: n.mayExit(s.grant),
			Transit:   s.transit,
			Carriage:  s.carriage(),
		})
	}
	return out
}

func (s *live) carriage() string {
	if s.conn == nil {
		return "tcp"
	}
	st := s.conn.ConnectionState()
	out := "quic " + st.Version.String()
	if st.TLS.ECHAccepted {
		out += " ech"
	}
	if st.Used0RTT {
		out += " 0rtt"
	}
	return out
}

func (n *Node) seatsOf(id uint32, drop bool) []*live {
	n.mu.Lock()
	defer n.mu.Unlock()

	going := []*live{}
	for seat, s := range n.held {
		if seat != id && s.grant.Session != id && s.grant.Via != id {
			continue
		}
		going = append(going, s)
		if drop {
			delete(n.held, seat)
		}
	}
	return going
}

func (n *Node) Forget(id uint32) {
	for _, s := range n.seatsOf(id, true) {
		n.links.forget(s.grant.Seat)
		n.mu.Lock()
		ends := []func(){}
		conns := []*quic.Conn{}
		for at := s; at != nil; at = at.under {
			if at.end != nil {
				ends = append(ends, at.end)
			}
			if at.conn != nil {
				conns = append(conns, at.conn)
			}
		}
		n.mu.Unlock()
		if s.end == nil {
			n.pool.give(s.address)
		}
		for _, end := range ends {
			end()
		}
		for _, qc := range conns {
			qc.CloseWithError(0, "")
		}
	}
}

func (n *Node) Reset(id uint32) {
	for _, s := range n.seatsOf(id, false) {
		s.up.Store(0)
		s.down.Store(0)
		s.pktUp.Store(0)
		s.pktDown.Store(0)
	}
	n.tally.Range(func(k, v any) bool {
		if k.(totalKey).session == id {
			t := v.(*totals)
			t.up.Store(0)
			t.down.Store(0)
			t.pktUp.Store(0)
			t.pktDown.Store(0)
		}
		return true
	})
}

func (n *Node) quicConfig() *quic.Config {
	t := n.Tunables()
	return &quic.Config{
		MaxIncomingStreams:             t.MaxStreams,
		EnableDatagrams:                true,
		Allow0RTT:                      true,
		MaxIdleTimeout:                 t.IdleTimeout,
		KeepAlivePeriod:                t.KeepAlive,
		InitialStreamReceiveWindow:     t.StreamWindow,
		MaxStreamReceiveWindow:         t.MaxStreamWin,
		InitialConnectionReceiveWindow: t.ConnWindow,
		MaxConnectionReceiveWindow:     t.MaxConnWin,
	}
}

func (n *Node) Run(ctx context.Context) error {
	t := n.Tunables()

	addr, err := net.ResolveUDPAddr("udp", n.cfg.Listen)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", n.cfg.Listen, err)
	}
	udp, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", n.cfg.Listen, err)
	}
	defer udp.Close()

	udp.SetReadBuffer(t.SocketBuffer)
	udp.SetWriteBuffer(t.SocketBuffer)
	n.checkBuffers(udp, t.SocketBuffer)
	n.roomForTCP()

	defer n.links.closeAll()

	mux := http.NewServeMux()
	mux.Handle("/", n.site)
	mux.HandleFunc(AuthPath, n.serveAuth)
	mux.HandleFunc(ConnectIPPath, n.serveConnectIP(ctx))
	mux.HandleFunc(IPOverTCPPath, n.serveIPOverTCP(ctx))
	mux.HandleFunc(RPCPath, n.serveRPC)
	mux.HandleFunc(UpdatePath, n.serveUpdate)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plain := r.Method == http.MethodConnect && r.URL != nil && r.URL.Path == "" && r.Host != ""
		if plain {
			n.serveConnect(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})

	srv := &http3.Server{
		Handler:         handler,
		EnableDatagrams: true,
		TLSConfig:       n.cfg.TLS,
		QUICConfig:      n.quicConfig(),
		ConnContext: func(ctx context.Context, c *quic.Conn) context.Context {
			sctx := newSessionContext(ctx, c)
			go n.untilGone(c, sessionOf(sctx))
			return sctx
		},
	}

	go func() {
		<-ctx.Done()
		srv.Close()
	}()

	go n.serveSite(ctx, srv, handler)
	go n.sweepLinks(ctx)

	n.srv = srv

	n.cfg.Log("quic      listening on %s, authority %s", n.cfg.Listen, n.cfg.Authority)
	tr := &quic.Transport{Conn: udp, StatelessResetKey: n.cfg.Reset}
	defer tr.Close()
	ln, err := tr.ListenEarly(http3.ConfigureTLSConfig(n.cfg.TLS), n.quicConfig())
	if err != nil {
		return fmt.Errorf("listen %s: %w", n.cfg.Listen, err)
	}
	defer ln.Close()
	return srv.ServeListener(ln)
}

func (n *Node) carrier(r *http.Request, opening bool) (Grant, bool) {
	grant, ok := n.verified(r)
	if !ok || grant.Session == 0 || grant.Seat == 0 {
		return Grant{}, false
	}
	if grant.Peer {
		return grant, true
	}
	if n.cfg.Admit != nil && !n.cfg.Admit(grant, r.Header.Get(update.HeaderVersion), r.Header.Get(update.HeaderKind)) {
		return Grant{}, false
	}
	if opening && n.cfg.Seat != nil && !n.cfg.Seat(grant, r.Header.Get(HeaderDevice)) {
		return Grant{}, false
	}
	return grant, true
}

func (n *Node) serveAuth(w http.ResponseWriter, r *http.Request) {
	grant, ok := n.carrier(r, true)
	if !ok {
		n.refused.Add(1)
		n.site.ServeHTTP(w, r)
		return
	}

	route := settled(routeOf(r))
	sessionOf(r.Context()).steer(route)

	onSeat := func() {
		n.mu.Lock()
		s := n.held[grant.Seat]
		n.mu.Unlock()

		turned := false
		if s != nil {
			s.lastSeen.Store(time.Now().Unix())
			turned = s.steer(route)
		}

		if turned {
			n.cfg.Log("quic      %s now steers to %q", grant.Client, route)
			if !s.marked {
				n.links.forget(grant.Seat)
			}
		}
	}
	if qc := sessionOf(r.Context()).quic(); qc != nil && !handshaken(qc) {
		go func() {
			select {
			case <-qc.HandshakeComplete():
				if qc.Context().Err() == nil {
					onSeat()
				}
			case <-qc.Context().Done():
			}
		}()
	} else {
		onSeat()
	}

	for _, p := range carried(n.pool.stream(grant.Seat)) {
		w.Header().Add(HeaderAddr, p.String())
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *live) steer(route string) bool {
	if s.heading() == route {
		return false
	}
	s.route.Store(&route)
	if !s.marked {
		s.shutFlows()
	}
	return true
}

func (s *live) shutFlows() {
	s.flowMu.Lock()
	held := s.flows
	s.flows = nil
	s.flowMu.Unlock()

	for _, flows := range held {
		for _, shut := range flows {
			shut.Close()
		}
	}
}

func (s *live) heading() string {
	if held := s.route.Load(); held != nil {
		return *held
	}
	return ""
}

const markSlots = 1 << 17

func slotOf(v6 bool, port uint16) uint32 {
	if v6 {
		return 1<<16 | uint32(port)
	}
	return uint32(port)
}

type flowAt struct{ src, dst netip.AddrPort }

func flowOf(src, dst netip.AddrPort) flowAt {
	return flowAt{
		src: netip.AddrPortFrom(src.Addr().Unmap(), src.Port()),
		dst: netip.AddrPortFrom(dst.Addr().Unmap(), dst.Port()),
	}
}

const headsCap = 1 << 14

func (s *live) noteMark(pkt []byte, mark uint64) {
	src, dst, _, ok := ippkt.Flow(pkt)
	if !ok {
		return
	}
	at := flowOf(src, dst)

	s.flowMu.Lock()
	was, seen := s.heads[at]
	if seen && was == mark {
		s.flowMu.Unlock()
		return
	}
	if s.heads == nil || len(s.heads) >= headsCap {
		s.heads = map[flowAt]uint64{}
	}
	s.heads[at] = mark
	var held []io.Closer
	if seen {
		held = s.flows[at]
		delete(s.flows, at)
	}
	s.flowMu.Unlock()

	for _, shut := range held {
		shut.Close()
	}
}

func (s *live) leaves(src, dst netip.AddrPort) bool {
	s.flowMu.Lock()
	defer s.flowMu.Unlock()
	return s.heads[flowOf(src, dst)] == MarkEgress
}

func (s *live) holdFlow(src, dst netip.AddrPort, shut io.Closer) func() {
	at := flowOf(src, dst)

	s.flowMu.Lock()
	if s.flows == nil {
		s.flows = map[flowAt][]io.Closer{}
	}
	s.flows[at] = append(s.flows[at], shut)
	s.flowMu.Unlock()

	return func() {
		s.flowMu.Lock()
		defer s.flowMu.Unlock()
		held := s.flows[at]
		i := slices.Index(held, shut)
		if i < 0 {
			return
		}
		if held = slices.Delete(held, i, i+1); len(held) == 0 {
			delete(s.flows, at)
			return
		}
		s.flows[at] = held
	}
}

func (n *Node) serveSite(ctx context.Context, quicSrv *http3.Server, served http.Handler) {
	conf := n.cfg.TLS.Clone()
	conf.NextProtos = []string{"h2", "http/1.1"}

	listener, err := tls.Listen("tcp", n.cfg.Listen, conf)
	if err != nil {
		n.cfg.Log("site      no tcp on %s, the port answers only over quic: %v", n.cfg.Listen, err)
		return
	}
	defer listener.Close()

	site := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			quicSrv.SetQUICHeaders(w.Header())
			served.ServeHTTP(w, r)
		}),
		ConnContext: func(ctx context.Context, _ net.Conn) context.Context {
			return newSessionContext(ctx, nil)
		},
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          log.New(io.Discard, "", 0),
	}

	go func() {
		<-ctx.Done()
		site.Close()
	}()

	n.cfg.Log("site      https on tcp %s", n.cfg.Listen)
	site.Serve(listener)
}

func (s *live) where() string {
	if s.conn != nil && !s.transit {
		if addr := s.conn.RemoteAddr(); addr != nil {
			return addr.String()
		}
	}
	return s.peer
}

var v6Once sync.Once
var v6Held bool
var v6Allowed atomic.Bool

func AllowV6(on bool) { v6Allowed.Store(on) }

func HoldsV6() bool {
	if !v6Allowed.Load() {
		return false
	}
	v6Once.Do(func() {
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			return
		}
		for _, a := range addrs {
			prefix, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			addr, ok := netip.AddrFromSlice(prefix.IP)
			if !ok || !addr.Is6() || addr.Is4In6() {
				continue
			}
			if addr.IsGlobalUnicast() && !addr.IsPrivate() && !addr.IsLinkLocalUnicast() {
				v6Held = true
				return
			}
		}
	})
	return v6Held
}

func handshaken(qc *quic.Conn) bool {
	select {
	case <-qc.HandshakeComplete():
		return qc.Context().Err() == nil
	default:
		return false
	}
}
