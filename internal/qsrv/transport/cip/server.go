package cip

import (
	"net/http"

	connectip "github.com/quic-go/connect-ip-go"
	"github.com/yosida95/uritemplate/v3"
)

// ProxyHandler монтирует connect-ip прокси на path и отдаёт каждое установленное
// туннельное соединение в onConn.
//
// Хендлер апгрейдит входящий connect-ip запрос (Extended CONNECT) и вызывает
// onConn(*connectip.Conn). onConn НЕ должен блокировать HTTP-обработчик надолго:
// типично он запускает обслуживание туннеля (в модели B — прокладку в gVisor
// netstack) в отдельной горутине и возвращает; соединение остаётся живым после
// возврата хендлера.
//
// В боевом узле этот же mux несёт decoy на прочих путях: валидный токен →
// connect-ip прокси, иначе → decoy (авторизация невидима для DPI).
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
			return // Proxy уже выставил статус
		}
		onConn(conn)
	})
	return mux
}
