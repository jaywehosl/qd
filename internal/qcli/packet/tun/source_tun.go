//go:build linux || android

// Package tun — источник сырых IP-пакетов поверх TUN-устройства.
//
// На телефоне дескриптор приходит готовым: его открывает VpnService.Builder и
// передаёт внутрь. Пакеты те же, что отдаёт WinDivert на Windows, поэтому
// контракт packet.Source общий — движок не знает, откуда они пришли.
package tun

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"

	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
)

// Source читает и пишет сырые IP через дескриптор TUN.
type Source struct {
	fd  int
	mtu int

	dropped atomic.Uint64

	mu     sync.Mutex
	closed bool
	busy   sync.WaitGroup

	// Батч живёт до следующего Recv, как того требует контракт: буфер
	// переиспользуется, копирует тот, кому данные нужны дольше.
	buf   []byte
	batch []packet.Packet
}

const (
	// Пачка амортизирует переходы в ядро. Больше держать нет смысла: на телефоне
	// в очереди TUN редко стоит больше десятка пакетов сразу.
	maxBatch = 32
	slot     = 65600
)

// Open берёт дескриптор, уже открытый системой, и переводит его в неблокирующий
// режим: иначе добрать второй пакет из очереди нельзя — чтение повисло бы до
// следующего, и батч всегда был бы из одного.
func Open(fd int, mtu int) (*Source, error) {
	if fd < 0 {
		return nil, errors.New("tun: no descriptor")
	}
	if mtu <= 0 {
		mtu = 1500
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		return nil, fmt.Errorf("tun: nonblocking: %w", err)
	}

	return &Source{
		fd:    fd,
		mtu:   mtu,
		buf:   make([]byte, maxBatch*slot),
		batch: make([]packet.Packet, 0, maxBatch),
	}, nil
}

func (s *Source) Recv(ctx context.Context) ([]packet.Packet, error) {
	if !s.enter() {
		return nil, os.ErrClosed
	}
	defer s.busy.Done()

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := s.wait(ctx); err != nil {
			return nil, err
		}

		s.batch = s.batch[:0]
		for len(s.batch) < maxBatch {
			at := len(s.batch) * slot
			n, err := unix.Read(s.fd, s.buf[at:at+slot])
			if err != nil {
				if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
					break
				}
				if errors.Is(err, unix.EINTR) {
					continue
				}
				if len(s.batch) > 0 {
					return s.batch, nil
				}
				return nil, fmt.Errorf("tun: read: %w", err)
			}
			if n <= 0 {
				break
			}
			s.batch = append(s.batch, packet.Packet{
				Data: s.buf[at : at+n],
				Dir:  packet.Outbound,
			})
		}

		if len(s.batch) > 0 {
			return s.batch, nil
		}
	}
}

// wait ждёт, пока в устройстве появятся пакеты. Ждём короткими шагами, чтобы
// закрытый контекст замечался сразу, а не висел до первого пакета.
func (s *Source) wait(ctx context.Context) error {
	fds := []unix.PollFd{{Fd: int32(s.fd), Events: unix.POLLIN}}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := unix.Poll(fds, pollStep)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return fmt.Errorf("tun: poll: %w", err)
		}
		s.mu.Lock()
		shut := s.closed
		s.mu.Unlock()
		if shut {
			return os.ErrClosed
		}
		if n > 0 {
			return nil
		}
	}
}

const pollStep = 200

func (s *Source) Send(pkts []packet.Packet) error {
	if !s.enter() {
		return os.ErrClosed
	}
	defer s.busy.Done()

	for i := range pkts {
		if len(pkts[i].Data) == 0 {
			continue
		}
		for {
			_, err := unix.Write(s.fd, pkts[i].Data)
			if err == nil {
				break
			}
			if errors.Is(err, unix.EINTR) {
				continue
			}
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				// Очередь устройства полна: пакет теряем, как теряет его сеть.
				// Ждать здесь значит держать весь путь из-за одного пакета.
				s.dropped.Add(1)
				break
			}
			return fmt.Errorf("tun: write: %w", err)
		}
	}
	return nil
}

func (s *Source) MTU() int { return s.mtu }

func (s *Source) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	s.busy.Wait()

	if err := unix.Close(s.fd); err != nil && !errors.Is(err, os.ErrClosed) {
		return err
	}
	return nil
}

var _ packet.Source = (*Source)(nil)

func (s *Source) enter() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.busy.Add(1)
	return true
}

func (s *Source) Dropped() uint64 { return s.dropped.Load() }
