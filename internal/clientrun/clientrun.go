// Package clientrun — подъём туннеля на клиенте.
//
// Порядок здесь один на все платформы: дозвониться, взять у узла адрес,
// поднять резолвер, открыть источник пакетов, запустить датапуть. Различие
// ровно одно — откуда берутся пакеты: на Windows их даёт драйвер захвата, на
// телефоне и на macOS дескриптор устройства. Всё остальное совпадало дословно,
// включая цепочки закрытий на каждом отказе, и расходилось при правках.
package clientrun

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/jaywehosl/quic-diver/internal/clientdns"
	"github.com/jaywehosl/quic-diver/internal/qcli"
	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
)

type Plan struct {
	Dial qcli.Options
	// Wait — сколько ждать дозвона; 0 означает двадцать секунд.
	Wait time.Duration
	// DNS — свой резолвер клиента; nil означает обойтись системным.
	DNS *clientdns.Config
	// Source отдаёт источник пакетов под уже поднятый туннель: адрес выдаёт
	// узел, и открывать устройство раньше дозвона значит взять адрес наугад.
	Source func(context.Context, *qcli.Tunnel) (packet.Source, error)
	// Lost зовётся, когда датапуть встал сам, а не по просьбе.
	Lost func(error)
	Say  func(format string, args ...any)
}

// Carried — то, что подняли. Гасит вызывающий: у каждого клиента своя уборка,
// и сводить её в одну было бы натяжкой.
type Carried struct {
	Live     *qcli.Tunnel
	DNS      *clientdns.Resolver
	Source   packet.Source
	Assigned netip.Prefix
	Endpoint string
	// Quit валит датапуть, Halt гасит резолвер, Gone закрывается, когда
	// датапуть действительно встал.
	Quit context.CancelFunc
	// Ctx умирает вместе с датапутём: сторожа вешать на него, а не на свой.
	Ctx  context.Context
	Halt chan struct{}
	Gone chan struct{}
}

const defaultWait = 20 * time.Second

func Carry(ctx context.Context, p Plan) (*Carried, error) {
	if len(p.Dial.Endpoints) == 0 {
		return nil, errors.New("no entrypoint to dial")
	}
	if p.Source == nil {
		return nil, errors.New("no source of packets")
	}
	wait := p.Wait
	if wait <= 0 {
		wait = defaultWait
	}

	began := time.Now()
	round, quit := context.WithCancel(ctx)

	// Всё, что уже открыто, закрывается одним списком: раньше каждая ветка
	// отказа несла свою цепочку, и стоило добавить шаг — одна из них отставала.
	undo := []func(){quit}
	give := func(err error) (*Carried, error) {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
		return nil, err
	}

	var dns *clientdns.Resolver
	if p.DNS != nil {
		made, err := clientdns.New(*p.DNS)
		if err != nil {
			return give(fmt.Errorf("dns: %w", err))
		}
		dns = made
		undo = append(undo, func() { dns.Close() })
		p.Dial.Resolver = dns.Addr()
	}

	dialCtx, dialStop := context.WithTimeout(round, wait)
	live, err := qcli.Dial(dialCtx, p.Dial)
	dialStop()
	if err != nil {
		return give(err)
	}
	undo = append(undo, func() { live.Close() })

	assigned := live.Assigned()
	if len(assigned) == 0 {
		return give(errors.New("the node assigned no address"))
	}
	if dns != nil {
		dns.SetNode(live.Endpoint())
	}

	src, err := p.Source(round, live)
	if err != nil {
		return give(err)
	}

	out := &Carried{
		Live: live, DNS: dns, Source: src,
		Assigned: assigned[0], Endpoint: live.Endpoint(),
		Quit: quit, Ctx: round, Halt: make(chan struct{}), Gone: make(chan struct{}),
	}

	go func() {
		defer close(out.Gone)
		defer src.Close()

		err := live.Run(round, src)
		if round.Err() != nil {
			return
		}
		if err == nil {
			err = errors.New("the data path stopped")
		}
		out.tell(p.Say, "carry: stopped: %v", err)
		if p.Lost != nil {
			go p.Lost(err)
		}
	}()

	if dns != nil {
		go dns.Serve(out.Halt)
		go dns.KeepWarm(out.Halt)
	}

	out.tell(p.Say, "carry: up in %d ms through %s, node gave %s",
		time.Since(began).Milliseconds(), out.Endpoint, out.Assigned)
	return out, nil
}

func (c *Carried) tell(say func(string, ...any), format string, args ...any) {
	if say != nil {
		say(format, args...)
	}
}
