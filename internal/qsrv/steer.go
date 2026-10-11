package qsrv

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/quic-go/quic-go/http3"
)

const steerRest = 20 * time.Second

func (n *Node) steerExit(ctx context.Context, seat, session uint32) (*http3.ClientConn, string, error) {
	if time.Now().UnixNano() < n.exitRest.Load() {
		return nil, "", fmt.Errorf("the exit nodes did not answer a moment ago")
	}
	cc, endpoint, err := n.raceExit(ctx, AnyExit, seat, session)
	if err != nil {
		n.exitRest.Store(time.Now().Add(steerRest).UnixNano())
	}
	return cc, endpoint, err
}

func (n *Node) AskExit(ctx context.Context, seat, session uint32, op string, body []byte) ([]byte, error) {
	cc, endpoint, err := n.steerExit(ctx, seat, session)
	if err != nil {
		return nil, err
	}
	at := where{endpoint, seat}
	n.links.hold(at)
	defer n.links.release(at)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://"+endpoint+RPCPath+op, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	rsp, err := cc.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s: %s", endpoint, op, rsp.Status)
	}
	return io.ReadAll(io.LimitReader(rsp.Body, maxAsk))
}
