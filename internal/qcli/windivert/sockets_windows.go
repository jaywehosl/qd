//go:build windows

package windivert

import (
	"context"
	"fmt"

	"golang.org/x/sys/windows"
)

type Sockets struct {
	h windows.Handle
}

func WatchSockets(dllPath string) (*Sockets, error) {
	if err := Load(dllPath); err != nil {
		return nil, err
	}
	filter := "outbound and (event == CONNECT or event == BIND or event == CLOSE)"
	h, err := open(filter, LayerSocket, 0, FlagSniff|FlagRecvOnly)
	if err != nil {
		return nil, fmt.Errorf("socket layer: %w", err)
	}
	return &Sockets{h: h}, nil
}

func (s *Sockets) Watch(ctx context.Context, took func(event uint8, data SocketData)) error {
	addrs := make([]Address, BatchMax)
	var none [1]byte

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, count, err := recvEx(s.h, none[:], addrs)
		if err != nil {
			return err
		}
		for i := uint(0); i < count; i++ {
			a := &addrs[i]
			took(a.Event(), a.Socket())
		}
	}
}

func (s *Sockets) Close() error {
	_ = shutdown(s.h, ShutdownBoth)
	return closeHandle(s.h)
}
