package localapi

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type Push struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type Feed func() []Push

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	allowed := s.origins
	feed := s.feed
	s.mu.RUnlock()

	if !s.holds(r.URL.Query().Get("t")) {
		http.Error(w, "", http.StatusUnauthorized)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return allowed[r.Header.Get("Origin")] },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		if feed != nil {
			for _, push := range feed() {
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteJSON(push); err != nil {
					return
				}
			}
		}
		select {
		case <-closed:
			return
		case <-ticker.C:
		}
	}
}
