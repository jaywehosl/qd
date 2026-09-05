// Package netstack — серверный конец модели B: gVisor netstack узла.
//
// Разворачивает сырые IP-пакеты из connect-ip туннеля, терминирует TCP/UDP в
// userspace-стеке и форвардит наружу — напрямую в интернет (direct) или в
// upstream-узел (chain). Обратный трафик стек сериализует в IP-пакеты и пишет
// назад в туннель. Это единственное место на узле, где живёт полноценный TCP/IP.
//
// NIC работает в promiscuous+spoofing режиме и держит маршрут по умолчанию, чтобы
// принимать соединения на ЛЮБОЙ адрес назначения (клиент ходит в произвольные
// хосты). Выбор direct/chain делает Dialer — туда позже придёт routing.Router.
package netstack

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/icmp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"gvisor.dev/gvisor/pkg/waiter"
)

const (
	nicID = 1
	// channelMTU занижен до 1280 (IPv6 min MTU): узел объявляет MSS ≤1240 и
	// клиенту, и удалённым серверам, поэтому ни один сегмент не превышает ёмкость
	// connect-ip датаграммы (QUIC datagram). Иначе крупные пакеты отбрасываются.
	channelMTU  = 1280
	maxInFlight = 2048
	dialTimeout = 15 * time.Second
)

// Tunnel — двусторонний канал сырых IP-пакетов (серверный connect-ip Conn).
type Tunnel interface {
	ReadPacket(b []byte) (int, error)
	WritePacket(b []byte) (icmp []byte, err error)
}

// Dialer выходит наружу с узла: direct (реальная сеть) или chain (upstream-узел).
type Dialer interface {
	DialTCP(ctx context.Context, dst netip.AddrPort) (net.Conn, error)
	DialUDP(ctx context.Context, dst netip.AddrPort) (net.Conn, error)
}

// Stack — netstack узла.
type Stack struct {
	opened func(port uint16, shut io.Closer)

	stack    *stack.Stack
	ep       *channel.Endpoint
	dialer   Dialer
	flows    sync.Map
	nextFlow atomic.Uint64
	mtu      int
}

// NewWithMTU — как New, но с явным MTU канала.
//
// Узлу нужен MTU туннеля (пакеты уедут в датаграммах), а клиентскому стеку — MTU
// локальной сети: он общается с ОС через инжект, и мелкий MTU только дробит поток
// на лишние пакеты.
func NewWithMTU(d Dialer, mtu int) (*Stack, error) {
	return newStack(d, mtu)
}

// New поднимает стек с TCP/UDP-форвардерами, направляющими трафик через d.
func New(d Dialer) (*Stack, error) {
	return newStack(d, channelMTU)
}

func newStack(d Dialer, mtu int) (*Stack, error) {
	s := stack.New(stack.Options{
		NetworkProtocols: []stack.NetworkProtocolFactory{
			ipv4.NewProtocol, ipv6.NewProtocol,
		},
		TransportProtocols: []stack.TransportProtocolFactory{
			tcp.NewProtocol, udp.NewProtocol, icmp.NewProtocol4, icmp.NewProtocol6,
		},
	})
	// Буферы TCP заметно больше дефолтных gVisor: стек между CONNECT-стримом и
	// приложением, и на дефолтах он становился узким местом (изолированный стрим
	// давал 645 Мбит, полный тракт — 300-400, при том что CPU стека мизерный).
	sndBuf := tcpip.TCPSendBufferSizeRangeOption{Min: 4 << 10, Default: 4 << 20, Max: 16 << 20}
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &sndBuf); err != nil {
		return nil, fmt.Errorf("tcp send buffer: %v", err)
	}
	rcvBuf := tcpip.TCPReceiveBufferSizeRangeOption{Min: 4 << 10, Default: 4 << 20, Max: 16 << 20}
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &rcvBuf); err != nil {
		return nil, fmt.Errorf("tcp recv buffer: %v", err)
	}

	ep := channel.New(4096, uint32(mtu), "")
	// Источник доверенный (наш перехват / наш туннель), а у перехваченных
	// исходящих контрольные суммы не досчитаны — их считает NIC (offload), а
	// WinDivert перехватывает раньше: в заголовке остаётся 0x0000. Без этой
	// capability стек бракует ВСЕ такие пакеты как malformed (nic.go выставляет
	// pkt.RXChecksumValidated именно отсюда, поэтому флаг на пакете бесполезен).
	ep.LinkEPCapabilities = stack.CapabilityRXChecksumOffload
	if err := s.CreateNIC(nicID, ep); err != nil {
		return nil, fmt.Errorf("create nic: %v", err)
	}
	if err := s.SetPromiscuousMode(nicID, true); err != nil {
		return nil, fmt.Errorf("promiscuous: %v", err)
	}
	if err := s.SetSpoofing(nicID, true); err != nil {
		return nil, fmt.Errorf("spoofing: %v", err)
	}
	s.SetRouteTable([]tcpip.Route{
		{Destination: header.IPv4EmptySubnet, NIC: nicID},
		{Destination: header.IPv6EmptySubnet, NIC: nicID},
	})

	st := &Stack{stack: s, ep: ep, dialer: d, mtu: mtu}

	tcpFwd := tcp.NewForwarder(s, 0, maxInFlight, st.handleTCP)
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpFwd.HandlePacket)
	udpFwd := udp.NewForwarder(s, st.handleUDP)
	s.SetTransportProtocolHandler(udp.ProtocolNumber, udpFwd.HandlePacket)

	return st, nil
}

// DebugStats — счётчики стека: видно, доходят ли пакеты до IP/TCP-слоя и почему
// отбрасываются (битая сумма, чужой адрес, мусор).
func (s *Stack) DebugStats() string {
	st := s.stack.Stats()
	return fmt.Sprintf(
		"IP rcvd=%d malformed=%d badDst=%d disp=%d noRoute=%d | TCP rcvd=%d valid=%d invalid=%d csumErr=%d listenDrop=%d | UDP rcvd=%d unknown=%d | ICMP v4in=%d",
		st.IP.PacketsReceived.Value(),
		st.IP.MalformedPacketsReceived.Value(),
		st.IP.InvalidDestinationAddressesReceived.Value(),
		st.IP.PacketsDelivered.Value(),
		st.IP.OutgoingPacketErrors.Value(),
		st.TCP.SegmentsSent.Value(),
		st.TCP.ValidSegmentsReceived.Value(),
		st.TCP.InvalidSegmentsReceived.Value(),
		st.TCP.ChecksumErrors.Value(),
		st.TCP.ListenOverflowSynDrop.Value(),
		st.UDP.PacketsReceived.Value(),
		st.UDP.UnknownPortErrors.Value(),
		st.ICMP.V4.PacketsReceived.Invalid.Value())
}

// Run гоняет обмен туннель↔стек до отмены ctx. ingress держит горутину вызова,
// egress — отдельную.
func (s *Stack) Run(ctx context.Context, t Tunnel) error {
	go s.egress(ctx, t)
	return s.ingress(ctx, t)
}

// ingress: IP-пакеты из туннеля → стек.
func (s *Stack) ingress(ctx context.Context, t Tunnel) error {
	// Буфер на максимальный IP-пакет, а не на MTU: на клиенте перехват идёт ДО
	// сегментации (Windows LSO отдаёт пакеты до 64 КБ), и короткий буфер молча
	// обрезал бы их — стек считал бы такие пакеты malformed.
	buf := make([]byte, 65536+header.IPv4MaximumHeaderSize)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := t.ReadPacket(buf)
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		var proto tcpip.NetworkProtocolNumber
		switch buf[0] >> 4 {
		case 4:
			proto = header.IPv4ProtocolNumber
		case 6:
			proto = header.IPv6ProtocolNumber
		default:
			continue
		}
		// Копия: buf переиспользуется на следующей итерации.
		data := make([]byte, n)
		copy(data, buf[:n])
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(data),
		})
		s.ep.InjectInbound(proto, pkt)
		pkt.DecRef()
	}
}

// egress: IP-пакеты из стека → туннель (ответы клиенту).
func (s *Stack) egress(ctx context.Context, t Tunnel) {
	var errs uint64
	for {
		pkt := s.ep.ReadContext(ctx)
		if pkt == nil {
			return // ctx отменён / стек закрыт
		}
		b := pkt.ToBuffer()
		_, err := t.WritePacket(b.Flatten())
		pkt.DecRef()
		if err != nil {
			// Сбой отправки ОДНОГО пакета — не повод глушить весь обратный путь.
			// Раньше здесь стоял return: первая же временная ошибка инжекта
			// навсегда останавливала доставку ответов, и снаружи это выглядело
			// как «сеть внезапно умерла» (пакеты идут, ответы — нет).
			errs++
			if errs <= 3 || errs%1000 == 0 {
				log.Printf("netstack: egress: %v (ошибок: %d, продолжаю)", err, errs)
			}
		}
	}
}

// handleTCP терминирует TCP-соединение в стеке и склеивает его с реальным
// исходящим соединением (direct/chain).
func (s *Stack) handleTCP(r *tcp.ForwarderRequest) {
	id := r.ID()
	dst := netip.AddrPortFrom(toNetip(id.LocalAddress), id.LocalPort)

	src := netip.AddrPortFrom(toNetip(id.RemoteAddress), id.RemotePort)

	ctx, cancel := context.WithTimeout(fromFlow(context.Background(), src, dst, false), dialTimeout)
	outbound, err := s.dialer.DialTCP(ctx, dst)
	cancel()
	if err != nil {
		log.Printf("netstack: dial %s: %v (RST инициатору)", dst, err)
		r.Complete(true) // RST инициатору
		return
	}

	var wq waiter.Queue
	ep, tcperr := r.CreateEndpoint(&wq)
	if tcperr != nil {
		log.Printf("netstack: CreateEndpoint %s: %v", dst, tcperr)
		outbound.Close()
		r.Complete(true)
		return
	}
	r.Complete(false)

	inbound := gonet.NewTCPConn(&wq, ep)
	held := s.keepFlow(Flow{Src: src, Dst: dst}, shutBoth{inbound, outbound})
	go func() {
		pipe(inbound, outbound)
		s.dropFlow(held)
	}()
}

// handleUDP терминирует UDP-флоу и склеивает с реальным исходящим сокетом.
func (s *Stack) handleUDP(r *udp.ForwarderRequest) {
	id := r.ID()
	dst := netip.AddrPortFrom(toNetip(id.LocalAddress), id.LocalPort)

	src := netip.AddrPortFrom(toNetip(id.RemoteAddress), id.RemotePort)

	outbound, err := s.dialer.DialUDP(fromFlow(context.Background(), src, dst, true), dst)
	if err != nil {
		log.Printf("netstack: udp dial %s: %v (пакет уронен)", dst, err)
		return
	}
	var wq waiter.Queue
	ep, tcperr := r.CreateEndpoint(&wq)
	if tcperr != nil {
		outbound.Close()
		return
	}
	inbound := gonet.NewUDPConn(s.stack, &wq, ep)
	shut := shutBoth{inbound, outbound}
	if s.opened != nil {
		s.opened(id.RemotePort, shut)
	}
	held := s.keepFlow(Flow{Src: src, Dst: dst, UDP: true}, shut)
	go func() {
		pipe(inbound, outbound)
		s.dropFlow(held)
	}()
}

// OnFlow вызывается при открытии UDP-флоу: порт источника и способ его закрыть.
// Нужен вызывающему, чтобы оборвать один флоу, не трогая остальные.
func (s *Stack) OnFlow(fn func(port uint16, shut io.Closer)) { s.opened = fn }

// keepFlow заносит живой разговор в реестр. Правило маршрутизации решается один
// раз при дозвоне, поэтому изменить дорогу уже открытому флоу нельзя — его надо
// оборвать, чтобы приложение набрало заново. Без реестра рвать было нечего:
// снаружи оставался только грубый разрыв в стеке ОС, а на телефоне и его нет.
func (s *Stack) keepFlow(f Flow, shut io.Closer) uint64 {
	id := s.nextFlow.Add(1)
	s.flows.Store(id, liveFlow{flow: f, shut: shut})
	return id
}

func (s *Stack) dropFlow(id uint64) { s.flows.Delete(id) }

// ShutFlows обрывает флоу, на которые указал take. Возвращает, сколько оборвал.
func (s *Stack) ShutFlows(take func(Flow) bool) int {
	shut := 0
	s.flows.Range(func(id, held any) bool {
		one := held.(liveFlow)
		if !take(one.flow) {
			return true
		}
		s.flows.Delete(id)
		one.shut.Close()
		shut++
		return true
	})
	return shut
}

type liveFlow struct {
	flow Flow
	shut io.Closer
}

type shutBoth struct{ a, b net.Conn }

func (s shutBoth) Close() error {
	s.a.Close()
	return s.b.Close()
}

// pipe качает данные в обе стороны и корректно закрывает половинки.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		} else {
			_ = dst.Close()
		}
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	<-done
	a.Close()
	b.Close()
}

func toNetip(a tcpip.Address) netip.Addr {
	if a.Len() == 4 {
		return netip.AddrFrom4(a.As4())
	}
	return netip.AddrFrom16(a.As16())
}

// NetDialer — прямой выход в реальную сеть узла (direct). Chain-реализация Dialer
// добавится отдельно (dial в upstream-узел).
type NetDialer struct {
	D net.Dialer
}

func (n NetDialer) DialTCP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	return n.D.DialContext(ctx, "tcp", dst.String())
}

func (n NetDialer) DialUDP(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	return n.D.DialContext(ctx, "udp", dst.String())
}

var _ Dialer = NetDialer{}

func (s *Stack) Reset(t Tunnel, drain time.Duration) {
	s.stack.Close()

	deadline := time.Now().Add(drain)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		pkt := s.ep.ReadContext(ctx)
		cancel()
		if pkt == nil {
			continue
		}
		b := pkt.ToBuffer()
		_, _ = t.WritePacket(b.Flatten())
		pkt.DecRef()
	}
	s.stack.Wait()
}

type flowKey struct{}

type Flow struct {
	Src netip.AddrPort
	Dst netip.AddrPort
	UDP bool
}

func fromFlow(ctx context.Context, src, dst netip.AddrPort, udp bool) context.Context {
	return context.WithValue(ctx, flowKey{}, Flow{Src: src, Dst: dst, UDP: udp})
}

func FlowOf(ctx context.Context) (Flow, bool) {
	f, ok := ctx.Value(flowKey{}).(Flow)
	return f, ok
}
