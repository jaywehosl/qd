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

	"github.com/jaywehosl/qd/internal/qsrv"
	"github.com/jaywehosl/qd/internal/update"
)

func (d *Dialer) Ask(endpoint, op, auth string, body any, out any) error {
	cc, sign, err := d.conn(endpoint)
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
	sign(req.Header)
	update.Stamp(req.Header)
	if auth != "" {
		req.Header.Set(qsrv.HeaderAuth, auth)
	}

	rsp, err := cc.RoundTrip(req)
	if err != nil {
		if ctx.Err() == nil || d.missed(endpoint) {
			d.drop(endpoint)
		}
		return fmt.Errorf("%s: %w", op, err)
	}
	defer rsp.Body.Close()
	d.answered(endpoint)

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

func (d *Dialer) Raw(endpoint, op, auth string, body any) (json.RawMessage, error) {
	var out json.RawMessage
	if err := d.Ask(endpoint, op, auth, body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func waitFor(op string) time.Duration {
	switch {
	case strings.HasPrefix(op, "db."):
		return heavyWait
	case op == "dns":
		return dnsWait
	case quickOps[op]:
		return quickWait
	default:
		return askWait
	}
}

var quickOps = map[string]bool{
	"hello": true, "whoami": true, "join": true, "bye": true,
}

const (
	quickWait = 3 * time.Second
	dnsWait   = 4 * time.Second
	askWait   = 10 * time.Second
	heavyWait = 2 * time.Minute
)

func (d *Dialer) Open(endpoint, path string) (io.ReadCloser, error) {
	cc, sign, err := d.conn(endpoint)
	if err != nil {
		return nil, err
	}
	ctx, stop := context.WithTimeout(context.Background(), fetchWait)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+endpoint+path, nil)
	if err != nil {
		stop()
		return nil, err
	}
	sign(req.Header)
	update.Stamp(req.Header)

	rsp, err := cc.RoundTrip(req)
	if err != nil {
		stop()
		return nil, err
	}
	if rsp.StatusCode != http.StatusOK {
		rsp.Body.Close()
		stop()
		return nil, fmt.Errorf("%s: %s", path, rsp.Status)
	}
	return stopping{ReadCloser: rsp.Body, stop: stop, size: rsp.ContentLength}, nil
}

type stopping struct {
	io.ReadCloser
	stop context.CancelFunc
	size int64
}

func (s stopping) Close() error {
	err := s.ReadCloser.Close()
	s.stop()
	return err
}

const fetchWait = 15 * time.Minute

func (s stopping) Size() int64 { return s.size }
