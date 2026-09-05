package qsrv

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// HeaderAuth — кто спрашивает: uuid администратора или клиента. Токен сети
// (Qd-Token) пускает в дверь, этот заголовок говорит, что человеку позволено.
const HeaderAuth = "Qd-Auth"

// maxAsk — потолок на тело запроса. Перелив базы идёт этим же путём, поэтому
// щедрый: гонять базу кусками по 800 байт заставляла датаграмма, а не здравый
// смысл.
const maxAsk = 64 << 20

// serveRPC — управление одним запросом: POST /qd/rpc/<операция>, тело и ответ
// JSON. Мультиплексирование, порядок и надёжность обеспечивает QUIC, шифрование
// — TLS; своего слоя поверх не нужно.
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
