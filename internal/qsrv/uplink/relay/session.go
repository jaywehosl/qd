package relay

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	marker        = "qd1:"
	keepAlive     = "qd0:ka"
	kaEvery       = 8 * time.Second
	writeWait     = 10 * time.Second
	defaultOrigin = "https://docs.datacloudmail.ru"
	maxReconnect  = 999999
	browserUA     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36"
)

type Link struct {
	Authority string `json:"authority"`
	Weblink   string `json:"weblink"`
}

const publicBase = "https://cloud.mail.ru/public/"

func Doc(weblink string) string {
	if i := strings.Index(weblink, "/public/"); i >= 0 {
		weblink = weblink[i+len("/public/"):]
	}
	return strings.Trim(weblink, "/ ")
}

func Weblink(doc string) string { return publicBase + Doc(doc) }

type Config struct {
	Public string
	Token  string
	DocID  string
	Keep   func(fd uintptr)
}

type Session struct {
	cfg Config
	Log func(string, ...any)

	running   atomic.Bool
	connected atomic.Bool

	mu   sync.Mutex
	conn *websocket.Conn

	queue chan []byte

	recvMu sync.RWMutex
	recv   func([]byte)

	peersMu sync.Mutex
	peers   map[netip.Addr]struct{}

	userID string
}

func New(cfg Config) *Session {
	return &Session{
		cfg:    cfg,
		userID: fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000),
		queue:  make(chan []byte, 1024),
		peers:  map[netip.Addr]struct{}{},
	}
}

var Lookup func(host string) []netip.Addr

var yandex = []string{"77.88.8.8:53", "77.88.8.1:53"}

func (s *Session) dialNote(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: 15 * time.Second}
	targets := []string{addr}
	if s.cfg.Keep != nil {
		keep := s.cfg.Keep
		control := func(_, _ string, rc syscall.RawConn) error {
			return rc.Control(keep)
		}
		d.Control = control
		d.Resolver = yandexResolver(control)
		targets = s.lookup(addr)
	}

	var c net.Conn
	var err error
	for _, target := range targets {
		if c, err = d.DialContext(ctx, network, target); err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	if ap, e := netip.ParseAddrPort(c.RemoteAddr().String()); e == nil {
		s.peersMu.Lock()
		s.peers[ap.Addr()] = struct{}{}
		s.peersMu.Unlock()
	}
	return c, nil
}

func (s *Session) lookup(addr string) []string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || Lookup == nil {
		return []string{addr}
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return []string{addr}
	}

	found := Lookup(host)
	if len(found) == 0 {
		s.logf("[relay] %s: the network gave no address, asking yandex dns", host)
		return []string{addr}
	}
	slices.SortStableFunc(found, func(a, b netip.Addr) int {
		if a.Is4() == b.Is4() {
			return 0
		}
		if a.Is4() {
			return -1
		}
		return 1
	})

	out := make([]string, 0, len(found))
	for _, ip := range found {
		out = append(out, net.JoinHostPort(ip.String(), port))
	}
	return out
}

func yandexResolver(control func(string, string, syscall.RawConn) error) *net.Resolver {
	var turn atomic.Uint32
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			server := yandex[(turn.Add(1)-1)%uint32(len(yandex))]
			return (&net.Dialer{Control: control}).DialContext(ctx, network, server)
		},
	}
}

func (s *Session) Ready(ctx context.Context) error {
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for !s.connected.Load() {
		select {
		case <-ctx.Done():
			return fmt.Errorf("the relay did not come up: %w", ctx.Err())
		case <-tick.C:
		}
	}
	return nil
}

func (s *Session) Peers() []netip.Addr {
	s.peersMu.Lock()
	defer s.peersMu.Unlock()
	out := make([]netip.Addr, 0, len(s.peers))
	for a := range s.peers {
		out = append(out, a)
	}
	return out
}

func (s *Session) OnReceive(cb func([]byte)) {
	s.recvMu.Lock()
	s.recv = cb
	s.recvMu.Unlock()
}

func (s *Session) callRecv(b []byte) {
	s.recvMu.RLock()
	cb := s.recv
	s.recvMu.RUnlock()
	if cb != nil {
		cb(b)
	}
}

func (s *Session) logf(f string, a ...any) {
	if s.Log != nil {
		s.Log(f, a...)
	}
}

func (s *Session) Start() error {
	s.running.Store(true)
	s.cfg.Public = Doc(s.cfg.Public)
	if s.cfg.Public == "" && s.cfg.Token == "" {
		return fmt.Errorf("relay: needs a Public link or a Token")
	}
	go s.writerLoop()
	go s.keepAliveLoop()
	s.connect(0)
	return nil
}

func (s *Session) Send(b []byte) error {
	select {
	case s.queue <- append([]byte(nil), b...):
		return nil
	default:
		return fmt.Errorf("relay: queue is full")
	}
}

func (s *Session) connect(attempt int) {
	if !s.running.Load() {
		return
	}

	go func() {
		if s.cfg.Public != "" && s.cfg.Token == "" {
			token, err := s.mint()
			if err != nil {
				s.logf("[relay] mint failed: %v", err)
				s.scheduleReconnect(attempt)
				return
			}
			s.cfg.Token = token
		}
		wcfg, err := decodeJWT(s.cfg.Token)
		if err != nil {
			s.logf("[relay] token decode: %v", err)
			s.cfg.Token = ""
			s.scheduleReconnect(attempt)
			return
		}
		host := strings.TrimPrefix(strings.TrimPrefix(wcfg.API, "https://"), "http://")
		wsURL := fmt.Sprintf("wss://%s/doc/%s/c/?EIO=4&transport=websocket", host, wcfg.Document.Key)

		dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, NetDialContext: s.dialNote}
		headers := http.Header{}
		headers.Set("User-Agent", browserUA)
		headers.Set("Origin", defaultOrigin)

		conn, resp, err := dialer.Dial(wsURL, headers)
		if err != nil {
			detail := ""
			if resp != nil {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
				resp.Body.Close()
				detail = fmt.Sprintf(" [%s] %s", resp.Status, strings.TrimSpace(string(body)))
			}
			s.logf("[relay] dial failed: %v%s", err, detail)
			if s.cfg.Public != "" {
				s.cfg.Token = ""
			}
			s.scheduleReconnect(attempt)
			return
		}

		s.mu.Lock()
		s.conn = conn
		s.mu.Unlock()

		if err := s.authenticate(conn); err != nil {
			s.logf("[relay] auth failed: %v", err)
			conn.Close()
			s.mu.Lock()
			if s.conn == conn {
				s.conn = nil
			}
			s.mu.Unlock()
			s.connected.Store(false)
			if s.cfg.Public != "" {
				s.cfg.Token = ""
			}
			s.scheduleReconnect(attempt)
			return
		}

		s.connected.Store(true)
		s.logf("[relay] coauthoring up, docid=%s", s.cfg.DocID)

		for s.running.Load() {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				s.logf("[relay] read error: %v", err)
				s.mu.Lock()
				if s.conn == conn {
					s.conn = nil
				}
				s.mu.Unlock()
				s.connected.Store(false)
				conn.Close()
				s.scheduleReconnect(0)
				return
			}
			s.handle(conn, msg)
		}
	}()
}

func (s *Session) mint() (string, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 20 * time.Second, Transport: &http.Transport{DialContext: s.dialNote}}
	pubURL := publicBase + s.cfg.Public

	if req, err := http.NewRequest("GET", pubURL, nil); err == nil {
		req.Header.Set("User-Agent", browserUA)
		if resp, err := client.Do(req); err == nil {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
		}
	}

	body := fmt.Sprintf(`{"public":%q,"home":""}`, s.cfg.Public)
	req, err := http.NewRequest("POST", "https://cloud.mail.ru/api/v4/r7/view", strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Version", "4")
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Origin", "https://cloud.mail.ru")
	req.Header.Set("Referer", pubURL+"?weblink="+s.cfg.Public)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("r7/view %d: %.140s", resp.StatusCode, raw)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", fmt.Errorf("r7/view: empty token")
	}
	return out.Token, nil
}

func (s *Session) authenticate(conn *websocket.Conn) error {
	cfg, err := decodeJWT(s.cfg.Token)
	if err != nil {
		return fmt.Errorf("jwtOpen: %w", err)
	}
	docKey := firstNonEmpty(s.cfg.DocID, cfg.Document.Key)
	userID := firstNonEmpty(s.userID, cfg.EditorConfig.User.ID)
	s.cfg.DocID = docKey
	mode := cfg.EditorConfig.Mode
	if mode == "" {
		mode = "edit"
	}
	view := mode == "view"
	anon := strings.HasPrefix(userID, "anon")

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, open, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("handshake read: %w", err)
	}
	if len(open) == 0 || open[0] != '0' {
		return fmt.Errorf("not an engine.io handshake: %.40s", open)
	}

	if err := s.write(conn, fmt.Sprintf(`40{"token":%q}`, s.cfg.Token)); err != nil {
		return err
	}
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("connect ack read: %w", err)
		}
		str := string(msg)
		if str == "2" {
			s.write(conn, "3")
			continue
		}
		if strings.HasPrefix(str, "40") {
			break
		}
	}
	conn.SetReadDeadline(time.Time{})

	auth := map[string]interface{}{
		"type":                "auth",
		"docid":               docKey,
		"documentCallbackUrl": cfg.EditorConfig.CallbackURL,
		"token":               "fghhfgsjdgfjs",
		"user": map[string]interface{}{
			"id": userID, "username": cfg.EditorConfig.User.Name,
			"firstname": nil, "lastname": nil, "indexUser": -1,
		},
		"editorType":         0,
		"lastOtherSaveTime":  -1,
		"block":              []interface{}{},
		"sessionId":          nil,
		"sessionTimeConnect": nil,
		"sessionTimeIdle":    0,
		"documentFormatSave": 65,
		"view":               view,
		"isCloseCoAuthoring": false,
		"openCmd": map[string]interface{}{
			"c": "open", "id": docKey, "userid": userID,
			"format": cfg.Document.FileType, "url": cfg.Document.URL,
			"title": cfg.Document.Title, "lcid": 25, "nobase64": true,
		},
		"lang":                  "ru",
		"mode":                  mode,
		"permissions":           cfg.Document.Permissions,
		"IsAnonymousUser":       anon,
		"timezoneOffset":        -180,
		"coEditingMode":         "fast",
		"jwtOpen":               s.cfg.Token,
		"supportAuthChangesAck": true,
	}
	return s.emit(conn, auth)
}

type jwtConfig struct {
	API      string `json:"api"`
	Document struct {
		Key         string          `json:"key"`
		URL         string          `json:"url"`
		Title       string          `json:"title"`
		FileType    string          `json:"fileType"`
		Permissions json.RawMessage `json:"permissions"`
	} `json:"document"`
	EditorConfig struct {
		CallbackURL string `json:"callbackUrl"`
		Mode        string `json:"mode"`
		User        struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"user"`
	} `json:"editorConfig"`
}

func decodeJWT(token string) (jwtConfig, error) {
	var cfg jwtConfig
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return cfg, fmt.Errorf("not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return cfg, err
	}
	return cfg, json.Unmarshal(payload, &cfg)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (s *Session) handle(conn *websocket.Conn, raw []byte) {
	text := string(raw)

	if text == "2" {
		s.write(conn, "3")
		return
	}
	if text == "3" || strings.HasPrefix(text, "40") {
		return
	}
	if !strings.HasPrefix(text, "42") {
		return
	}
	var evt []json.RawMessage
	if json.Unmarshal([]byte(text[2:]), &evt) != nil || len(evt) < 2 {
		return
	}
	var body struct {
		Type     string `json:"type"`
		Cursor   string `json:"cursor"`
		Messages []struct {
			Cursor string `json:"cursor"`
		} `json:"messages"`
	}
	if json.Unmarshal(evt[1], &body) != nil || body.Type != "cursor" {
		return
	}

	cursors := make([]string, 0, len(body.Messages)+1)
	if body.Cursor != "" {
		cursors = append(cursors, body.Cursor)
	}
	for _, m := range body.Messages {
		cursors = append(cursors, m.Cursor)
	}
	for _, c := range cursors {
		if !strings.HasPrefix(c, marker) {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(c[len(marker):])
		if err != nil {
			continue
		}
		s.callRecv(data)
	}
}

func (s *Session) writerLoop() {
	for s.running.Load() {
		select {
		case pkt := <-s.queue:
			s.sendCursor(marker + base64.StdEncoding.EncodeToString(pkt))
		case <-time.After(time.Second):
		}
	}
}

func (s *Session) keepAliveLoop() {
	tick := time.NewTicker(kaEvery)
	defer tick.Stop()
	for s.running.Load() {
		<-tick.C
		if s.connected.Load() {
			s.sendCursor(keepAlive)
		}
	}
}

func (s *Session) sendCursor(cursor string) {
	msg := map[string]interface{}{"type": "cursor", "cursor": cursor}
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return
	}
	if err := s.emit(conn, msg); err != nil {
		s.logf("[relay] write error: %v", err)
	}
}

func (s *Session) emit(conn *websocket.Conn, body interface{}) error {
	part, err := json.Marshal([]interface{}{"message", body})
	if err != nil {
		return err
	}
	return s.write(conn, "42"+string(part))
}

func (s *Session) write(conn *websocket.Conn, str string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	conn.SetWriteDeadline(time.Now().Add(writeWait))
	return conn.WriteMessage(websocket.TextMessage, []byte(str))
}

func (s *Session) scheduleReconnect(attempt int) {
	if !s.running.Load() || attempt >= maxReconnect {
		return
	}
	delay := 200 * time.Millisecond
	if attempt > 0 {
		delay = time.Duration(attempt) * 400 * time.Millisecond
	}
	if delay > 3*time.Second {
		delay = 3 * time.Second
	}
	time.Sleep(delay)
	s.connect(attempt + 1)
}

func (s *Session) Stop() error {
	s.running.Store(false)
	s.connected.Store(false)
	s.mu.Lock()
	if s.conn != nil {
		s.conn.Close()
	}
	s.mu.Unlock()
	return nil
}
