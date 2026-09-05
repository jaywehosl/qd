package localapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
)

const TokenHeader = "X-QD-Token"

type Server struct {
	mu      sync.RWMutex
	token   string
	addr    net.Addr
	origins map[string]bool

	page   http.Handler
	client http.Handler
	admin  http.Handler

	isAdmin func() bool
	index   func(token string) ([]byte, error)
	feed    Feed
}

func (s *Server) SetFeed(feed Feed) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.feed = feed
}

type Config struct {
	Page    http.Handler
	Client  http.Handler
	Admin   http.Handler
	IsAdmin func() bool
	Index   func(token string) ([]byte, error)
}

func New(cfg Config) (*Server, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}

	isAdmin := cfg.IsAdmin
	if isAdmin == nil {
		isAdmin = func() bool { return false }
	}

	return &Server{
		token:   token,
		origins: map[string]bool{},
		page:    cfg.Page,
		client:  cfg.Client,
		admin:   cfg.Admin,
		isAdmin: isAdmin,
		index:   cfg.Index,
	}, nil
}

const DefaultPort = 48120

func (s *Server) Listen(host string) (net.Listener, error) {
	return s.ListenOn(host, DefaultPort)
}

func listenNear(host string, port int) (net.Listener, error) {
	if port <= 0 {
		return net.Listen("tcp", net.JoinHostPort(host, "0"))
	}

	var last error
	for step := 0; step < 16; step++ {
		l, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port+step)))
		if err == nil {
			return l, nil
		}
		last = err
	}
	if l, err := net.Listen("tcp", net.JoinHostPort(host, "0")); err == nil {
		return l, nil
	}
	return nil, last
}

func (s *Server) ListenOn(host string, port int) (net.Listener, error) {
	l, err := listenNear(host, port)
	if err != nil {
		return nil, err
	}
	s.rememberOrigin(l.Addr())

	s.mu.Lock()
	s.addr = l.Addr()
	s.mu.Unlock()
	return l, nil
}

func (s *Server) URL() string {
	s.mu.RLock()
	addr := s.addr
	s.mu.RUnlock()
	if addr == nil {
		return ""
	}
	return fmt.Sprintf("http://%s/client", addr.String())
}

func (s *Server) Allow(origin string) {
	if origin == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.origins[origin] = true
}

func (s *Server) rememberOrigin(addr net.Addr) {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, h := range []string{host, "127.0.0.1", "localhost"} {
		s.origins["http://"+net.JoinHostPort(h, port)] = true
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/ws" {
		s.websocket(w, r)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/client/api/") || strings.HasPrefix(r.URL.Path, "/panel/api/") ||
		strings.HasPrefix(r.URL.Path, "/panel/setting/") {
		if !s.authorised(r) {
			http.Error(w, "", http.StatusUnauthorized)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/client/api/") {
			s.client.ServeHTTP(w, r)
			return
		}
		if !s.isAdmin() {
			reply(w, false, "this key does not administer any node")
			return
		}
		s.admin.ServeHTTP(w, r)
		return
	}

	if isNavigation(r) {
		s.servePage(w)
		return
	}

	s.page.ServeHTTP(w, r)
}

func isNavigation(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if strings.Contains(path.Base(r.URL.Path), ".") {
		return false
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

func (s *Server) servePage(w http.ResponseWriter) {
	s.mu.RLock()
	token := s.token
	s.mu.RUnlock()

	body, err := s.index(token)
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	body = append(body, []byte(keepToken)...)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(body)
}

func (s *Server) authorised(r *http.Request) bool {
	s.mu.RLock()
	token := s.token
	allowed := s.origins
	s.mu.RUnlock()

	if origin := r.Header.Get("Origin"); origin != "" && !allowed[origin] {
		return false
	}
	given := r.Header.Get(TokenHeader)
	return subtle.ConstantTimeCompare([]byte(given), []byte(token)) == 1
}

func (s *Server) Token() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
}

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func reply(w http.ResponseWriter, ok bool, msg string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": ok, "msg": msg, "obj": nil})
}

const keepToken = `<script>try{sessionStorage.setItem("qd.token",window.QD_TOKEN)}catch(e){}</script>`
