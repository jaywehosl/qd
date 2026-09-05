package qsrv

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	connectip "github.com/quic-go/connect-ip-go"
	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/yosida95/uritemplate/v3"

	"github.com/jaywehosl/quic-diver/internal/qsrv/server/decoy"
)

const (
	AuthPath      = "/qd/hello"
	ConnectIPPath = "/qd/ip"
	RPCPath       = "/qd/rpc/"

	defaultHops = 2
)

type Grant struct {
	Client    string
	AllowExit bool
	Session   uint32
}

type Tunables struct {
	MaxStreams    int64
	StreamWindow  uint64
	MaxStreamWin  uint64
	ConnWindow    uint64
	MaxConnWin    uint64
	IdleTimeout   time.Duration
	KeepAlive     time.Duration
	SocketBuffer  int
	MTU           int
	Brutal        int
	MaxPacketSize uint16
}

func DefaultTunables() Tunables {
	return Tunables{
		MaxStreams:   65536,
		StreamWindow: 2 << 20,
		MaxStreamWin: 6 << 20,
		ConnWindow:   3 << 20,
		MaxConnWin:   15 << 20,
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
	Pool      netip.Prefix

	TLS   *tls.Config
	Token string

	Verify func(raw string) (Grant, bool)
	Peers  func() []Peer
	Tune   func() Tunables

	Ask func(op string, body []byte, auth string) (any, error)
	Log func(format string, args ...any)
}

type Session struct {
	Session   uint32
	Client    string
	Address   netip.Prefix
	Peer      string
	Since     int64
	LastSeen  int64
	Up        uint64
	Down      uint64
	PktUp     uint64
	PktDown   uint64
	AllowExit bool
	Transit   bool
}

type live struct {
	grant   Grant
	address netip.Prefix
	peer    string
	conn    *quic.Conn
	since   int64
	transit bool

	route  atomic.Pointer[string]
	swap   chan struct{}
	marks  sync.Map
	marked atomic.Int64
	flows  sync.Map

	up, down       atomic.Uint64
	pktUp, pktDown atomic.Uint64
	lastSeen       atomic.Int64
}

type Node struct {
	cfg   Config
	tune  atomic.Pointer[Tunables]
	pool  *pool
	nat   *nat64
	links *links
	proxy *connectip.Proxy
	tmpl  *uritemplate.Template
	site  http.Handler

	turn     atomic.Uint64
	exitTag  atomic.Pointer[string]
	transits atomic.Uint64
	refused  atomic.Uint64

	mu   sync.Mutex
	held map[uint32]*live
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
		nat:   newNAT64(),
		links: newLinks(cfg.Token, cfg.SelfID, cfg.Log),
		proxy: &connectip.Proxy{},
		tmpl:  Template(cfg.Authority, ConnectIPPath),
		site:  decoy.Handler(),
		held:  map[uint32]*live{},
	}
	n.Retune(n.tunables())
	return n
}

func Template(authority, path string) *uritemplate.Template {
	return uritemplate.MustNew(fmt.Sprintf("https://%s%s", authority, path))
}

func (n *Node) tunables() Tunables {
	if n.cfg.Tune == nil {
		return DefaultTunables()
	}
	t := n.cfg.Tune()
	base := DefaultTunables()
	if t.MaxStreams <= 0 {
		t.MaxStreams = base.MaxStreams
	}
	if t.StreamWindow == 0 {
		t.StreamWindow = base.StreamWindow
	}
	if t.MaxStreamWin == 0 {
		t.MaxStreamWin = base.MaxStreamWin
	}
	if t.ConnWindow == 0 {
		t.ConnWindow = base.ConnWindow
	}
	if t.MaxConnWin == 0 {
		t.MaxConnWin = base.MaxConnWin
	}
	if t.IdleTimeout == 0 {
		t.IdleTimeout = base.IdleTimeout
	}
	if t.KeepAlive == 0 {
		t.KeepAlive = base.KeepAlive
	}
	if t.SocketBuffer == 0 {
		t.SocketBuffer = base.SocketBuffer
	}
	if t.MTU == 0 {
		t.MTU = base.MTU
	}
	return t
}

func (n *Node) Retune(t Tunables) {
	n.tune.Store(&t)
}

func (n *Node) Tunables() Tunables {
	if held := n.tune.Load(); held != nil {
		return *held
	}
	return DefaultTunables()
}

func (n *Node) SetToken(token string) {
	n.cfg.Token = token
	n.links.setToken(token)
}

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
	for id, s := range n.held {
		out = append(out, Session{
			Session:   id,
			Client:    s.grant.Client,
			Address:   s.address,
			Peer:      s.where(),
			Since:     s.since,
			LastSeen:  s.lastSeen.Load(),
			Up:        s.up.Load(),
			Down:      s.down.Load(),
			PktUp:     s.pktUp.Load(),
			PktDown:   s.pktDown.Load(),
			AllowExit: s.grant.AllowExit,
			Transit:   s.transit,
		})
	}
	return out
}

func (n *Node) Forget(id uint32) {
	n.mu.Lock()
	s := n.held[id]
	delete(n.held, id)
	n.mu.Unlock()

	if s != nil {
		n.pool.give(s.address)
	}
}

func (n *Node) Reset(id uint32) {
	n.mu.Lock()
	s := n.held[id]
	n.mu.Unlock()
	if s == nil {
		return
	}
	s.up.Store(0)
	s.down.Store(0)
	s.pktUp.Store(0)
	s.pktDown.Store(0)
}

func (n *Node) quicConfig() *quic.Config {
	t := n.Tunables()
	return &quic.Config{
		MaxIncomingStreams:             t.MaxStreams,
		EnableDatagrams:                true,
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

	defer n.links.closeAll()

	mux := http.NewServeMux()
	mux.Handle("/", n.site)
	mux.HandleFunc(AuthPath, n.serveAuth)
	mux.HandleFunc(ConnectIPPath, n.serveConnectIP(ctx))
	mux.HandleFunc(RPCPath, n.serveRPC)

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

	go n.serveSite(ctx, srv)
	go n.sweepLinks(ctx)

	n.cfg.Log("quic      listening on %s, authority %s", n.cfg.Listen, n.cfg.Authority)
	return srv.Serve(udp)
}

func (n *Node) admit(r *http.Request) (Grant, bool) {
	if n.cfg.Verify == nil {
		return Grant{}, false
	}
	return n.verified(r)
}

func (n *Node) carrier(r *http.Request) (Grant, bool) {
	grant, ok := n.admit(r)
	if !ok || grant.Session == 0 {
		return Grant{}, false
	}
	return grant, true
}

func (n *Node) serveAuth(w http.ResponseWriter, r *http.Request) {
	grant, ok := n.carrier(r)
	if !ok {
		n.refused.Add(1)
		n.site.ServeHTTP(w, r)
		return
	}

	route := settled(routeOf(r))
	sessionOf(r.Context()).steer(route)

	n.mu.Lock()
	turned := false
	if s := n.held[grant.Session]; s != nil {
		turned = s.steer(route)
	}
	n.mu.Unlock()

	if turned {
		n.cfg.Log("quic      %s now steers to %q", grant.Client, route)
		n.links.forget(grant.Session)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (n *Node) SetExitTag(tag string) {
	n.exitTag.Store(&tag)
}

func (n *Node) ExitTag() string {
	if held := n.exitTag.Load(); held != nil {
		return *held
	}
	return ""
}

func (n *Node) Probe(ctx context.Context, endpoint string) (int, error) {
	began := time.Now()
	if _, err := n.links.to(endpoint, 0).connect(ctx); err != nil {
		return -1, err
	}
	return int(time.Since(began).Milliseconds()), nil
}

func (s *live) steer(route string) bool {
	if s.heading() == route {
		return false
	}
	s.route.Store(&route)
	s.shutFlows()
	return true
}

func (s *live) shutFlows() {
	s.flows.Range(func(key, held any) bool {
		s.flows.Delete(key)
		if shut, ok := held.(io.Closer); ok {
			shut.Close()
		}
		return true
	})
	s.marks.Range(func(key, _ any) bool {
		s.marks.Delete(key)
		return true
	})
	s.marked.Store(0)
}

func (s *live) heading() string {
	if held := s.route.Load(); held != nil {
		return *held
	}
	return ""
}

func (s *live) noteMark(pkt []byte, mark uint64) {
	port, ok := sourcePort(pkt)
	if !ok {
		return
	}
	if s.markOf(port) == mark {
		return
	}
	if mark == MarkHere {
		s.marks.Delete(port)
		s.marked.Add(-1)
	} else {
		s.marks.Store(port, mark)
		if s.marked.Add(1) > markCeiling {
			s.forgetStaleMarks()
		}
	}
	s.shutFlow(port)
}

func (s *live) shutFlow(port uint16) {
	held, ok := s.flows.LoadAndDelete(port)
	if !ok {
		return
	}
	held.(io.Closer).Close()
}

func (s *live) holdFlow(port uint16, shut io.Closer) {
	if was, ok := s.flows.Swap(port, shut); ok {
		was.(io.Closer).Close()
	}
}

func (s *live) markOf(port uint16) uint64 {
	if held, ok := s.marks.Load(port); ok {
		return held.(uint64)
	}
	return MarkHere
}

func sourcePort(pkt []byte) (uint16, bool) {
	var proto byte
	var rest []byte
	switch {
	case len(pkt) >= 20 && pkt[0]>>4 == 4:
		head := int(pkt[0]&0x0f) * 4
		if head < 20 || len(pkt) < head+4 {
			return 0, false
		}
		proto, rest = pkt[9], pkt[head:]
	case len(pkt) >= 44 && pkt[0]>>4 == 6:
		proto, rest = pkt[6], pkt[40:]
	default:
		return 0, false
	}
	if proto != 6 && proto != 17 {
		return 0, false
	}
	return binary.BigEndian.Uint16(rest[0:2]), true
}

func (s *live) forgetStaleMarks() {
	s.marks.Range(func(k, _ any) bool {
		s.marks.Delete(k)
		return true
	})
	s.marked.Store(0)
}

const markCeiling = 4096

func (n *Node) serveSite(ctx context.Context, quicSrv *http3.Server) {
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
			n.site.ServeHTTP(w, r)
		}),
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
	if s.conn != nil {
		if addr := s.conn.RemoteAddr(); addr != nil {
			return addr.String()
		}
	}
	return s.peer
}

var v6Once sync.Once
var v6Held bool

func (n *Node) holdsV6() bool {
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
