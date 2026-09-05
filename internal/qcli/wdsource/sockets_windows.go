//go:build windows

package windivert

import (
	"context"
	"fmt"

	"golang.org/x/sys/windows"
)

// Sockets — наблюдатель SOCKET-слоя: кто открывает и кто закрывает исходящий
// флоу. NETWORK-слой знает только пакет, поэтому хозяина первого пакета
// приходилось искать в таблицах Windows задним числом; здесь он известен ещё до
// того, как этот пакет уйдёт.
type Sockets struct {
	h windows.Handle
}

// WatchSockets открывает наблюдение. Хэндл только слушает (Sniff|RecvOnly):
// ничего не задерживает и ничего не может уронить.
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

// Watch читает события до отмены ctx и отдаёт каждое в took.
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
