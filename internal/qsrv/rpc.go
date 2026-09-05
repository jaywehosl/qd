package qsrv

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const HeaderAuth = "Qd-Auth"

const maxAsk = 64 << 20

func (n *Node) serveRPC(w http.ResponseWriter, r *http.Request) {
	if _, ok := n.admit(r); !ok {
		n.refused.Add(1)
		n.site.ServeHTTP(w, r)
		return
	}
	if n.cfg.Ask == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	op := strings.TrimPrefix(r.URL.Path, RPCPath)
	if op == "" || strings.Contains(op, "/") {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxAsk))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	answer, err := n.cfg.Ask(op, body, r.Header.Get(HeaderAuth))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if answer == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if raw, ok := answer.(json.RawMessage); ok {
		w.Write(raw)
		return
	}
	json.NewEncoder(w).Encode(answer)
}
