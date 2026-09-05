package localapi

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type Push struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type Feed func() []Push

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(strings.ToLower(r.Header.Get("Upgrade")), "websocket") {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	allowed := s.origins
	feed := s.feed
	s.mu.RUnlock()

	if origin := r.Header.Get("Origin"); origin == "" || !allowed[origin] {
		http.Error(w, "", http.StatusForbidden)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	conn, buffered, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()

	sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + wsGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	if _, err := buffered.WriteString(
		"HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"); err != nil {
		return
	}
	buffered.Flush()

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		drain(buffered.Reader)
	}()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		if feed != nil {
			for _, push := range feed() {
				body, err := json.Marshal(push)
				if err != nil {
					continue
				}
				if err := writeText(conn, body); err != nil {
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

func writeText(conn net.Conn, payload []byte) error {
	header := []byte{0x81}
	switch n := len(payload); {
	case n < 126:
		header = append(header, byte(n))
	case n < 1<<16:
		header = append(header, 126, 0, 0)
		binary.BigEndian.PutUint16(header[2:], uint16(n))
	default:
		header = append(header, 127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(header[2:], uint64(n))
	}

	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

func drain(r *bufio.Reader) {
	head := make([]byte, 2)
	for {
		if _, err := io.ReadFull(r, head); err != nil {
			return
		}
		if head[0]&0x0f == 0x8 {
			return
		}

		masked := head[1]&0x80 != 0
		length := uint64(head[1] & 0x7f)
		switch length {
		case 126:
			ext := make([]byte, 2)
			if _, err := io.ReadFull(r, ext); err != nil {
				return
			}
			length = uint64(binary.BigEndian.Uint16(ext))
		case 127:
			ext := make([]byte, 8)
			if _, err := io.ReadFull(r, ext); err != nil {
				return
			}
			length = binary.BigEndian.Uint64(ext)
		}
		if masked {
			length += 4
		}
		if length > 1<<20 {
			return
		}
		if _, err := io.CopyN(io.Discard, r, int64(length)); err != nil {
			return
		}
	}
}
