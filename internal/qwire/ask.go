package qwire

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jaywehosl/quic-diver/internal/qsrv"
)

// Ask выполняет одну управляющую операцию: POST /qd/rpc/<op>, тело и ответ JSON.
//
// Здесь нет ни своих кадров с длиной, ни своего шифрования, ни номеров запросов
// с таблицей ожидающих, ни ретраев. Всё это было нужно, пока управление ехало
// UDP-датаграммами: там не было ни границ сообщений, ни порядка, ни шифра.
// Поверх QUIC каждый запрос — отдельный поток: он сам мультиплексируется, сам
// доставляется по порядку и уже зашифрован TLS.
func (d *Dialer) Ask(endpoint, op, auth string, body any, out any) error {
	cc, token, err := d.conn(endpoint)
	if err != nil {
		return err
	}

	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(raw)
	}

	ctx, stop := context.WithTimeout(context.Background(), waitFor(op))
	defer stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://"+endpoint+qsrv.RPCPath+op, payload)
	if err != nil {
		return err
	}
	req.Header.Set(qsrv.HeaderToken, token)
	if auth != "" {
		req.Header.Set(qsrv.HeaderAuth, auth)
	}

	rsp, err := cc.RoundTrip(req)
	if err != nil {
		d.drop(endpoint)
		return fmt.Errorf("%s: %w", op, err)
	}
	defer rsp.Body.Close()

	answer, err := io.ReadAll(rsp.Body)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	switch rsp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
	case http.StatusUnprocessableEntity:
		var said struct {
			Error string `json:"error"`
		}
		json.Unmarshal(answer, &said)
		if said.Error == "" {
			said.Error = rsp.Status
		}
		return fmt.Errorf("%s: %s", op, said.Error)
	default:
		return fmt.Errorf("%s: %s", op, rsp.Status)
	}

	if out == nil || len(answer) == 0 {
		return nil
	}
	return json.Unmarshal(answer, out)
}

// Raw — та же операция, но ответ отдаётся как есть: панель передаёт его наружу
// без разбора.
func (d *Dialer) Raw(endpoint, op, auth string, body any) (json.RawMessage, error) {
	var out json.RawMessage
	if err := d.Ask(endpoint, op, auth, body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// waitFor — сколько ждать ответа. Разговор с узлом бывает трёх весов, и мерить
// их одной меркой нельзя: пока таймаут был общим и долгим, неответивший узел
// держал панель и клиент по двадцать секунд — а «задержка до узла» показывалась
// как ровно этот потолок.
func waitFor(op string) time.Duration {
	switch {
	case strings.HasPrefix(op, "db."):
		return heavyWait // перелив базы целиком
	case op == "dns":
		return dnsWait
	case quickOps[op]:
		return quickWait // живость и присутствие: не ответил быстро — считаем, что не ответил
	default:
		return askWait
	}
}

// quickOps — вопросы, которые обязаны отвечать мгновенно: ими меряют живость
// узла и отмечаются приход-уход клиента. Ждать их долго незачем — ответ, пришедший
// через двадцать секунд, уже никому не нужен.
var quickOps = map[string]bool{
	"hello": true, "whoami": true, "join": true, "bye": true,
}

const (
	quickWait = 3 * time.Second
	dnsWait   = 4 * time.Second
	askWait   = 10 * time.Second
	heavyWait = 2 * time.Minute
)
