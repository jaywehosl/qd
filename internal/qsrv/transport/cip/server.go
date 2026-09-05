package cip

import (
	"net/http"

	connectip "github.com/quic-go/connect-ip-go"
	"github.com/yosida95/uritemplate/v3"
)

func ProxyHandler(path string, tmpl *uritemplate.Template, onConn func(*connectip.Conn)) http.Handler {
	p := &connectip.Proxy{}
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		req, err := connectip.ParseRequest(r, tmpl)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		conn, err := p.Proxy(w, req)
		if err != nil {
			return
		}
		onConn(conn)
	})
	return mux
}
