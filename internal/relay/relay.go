package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/xtls/xray-core/app/dispatcher"
	"github.com/xtls/xray-core/app/proxyman"
	_ "github.com/xtls/xray-core/app/proxyman/outbound"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/proxy/vless"
	vlessout "github.com/xtls/xray-core/proxy/vless/outbound"
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/reality"
	"github.com/xtls/xray-core/transport/internet/splithttp"
	_ "github.com/xtls/xray-core/transport/internet/tcp"
	xtls "github.com/xtls/xray-core/transport/internet/tls"
	"github.com/xtls/xray-core/transport/internet/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/jaywehosl/qd/internal/vlesslink"
)

type Dialer struct {
	Link vlesslink.Link
	core *core.Instance
}

func New(l vlesslink.Link) (*Dialer, error) {
	stream := &internet.StreamConfig{ProtocolName: "tcp"}
	if l.XHTTP != nil {
		stream.ProtocolName = "splithttp"
		stream.TransportSettings = []*internet.TransportConfig{{
			ProtocolName: "splithttp",
			Settings:     serial.ToTypedMessage(xhttpOf(l.XHTTP)),
		}}
	}
	if l.WS != nil {
		stream.ProtocolName = "websocket"
		stream.TransportSettings = []*internet.TransportConfig{{
			ProtocolName: "websocket",
			Settings:     serial.ToTypedMessage(&websocket.Config{Host: l.WS.Host, Path: l.WS.Path, Ed: l.WS.Ed}),
		}}
	}
	switch l.Security {
	case "reality":
		secure(stream, &reality.Config{
			Fingerprint: l.Fingerprint,
			ServerName:  l.SNI,
			PublicKey:   l.PublicKey,
			ShortId:     l.ShortID,
			SpiderX:     l.SpiderX,
			SpiderY:     make([]int64, 10),
		})
	case "tls":
		secure(stream, &xtls.Config{
			ServerName:    l.SNI,
			Fingerprint:   l.Fingerprint,
			NextProtocol:  l.ALPN,
			AllowInsecure: l.Insecure,
		})
	}

	account := &vless.Account{Id: l.ID, Flow: l.Flow, Encryption: "none"}
	conf := &core.Config{
		App: []*serial.TypedMessage{
			serial.ToTypedMessage(&dispatcher.Config{}),
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
		},
		Outbound: []*core.OutboundHandlerConfig{{
			SenderSettings: serial.ToTypedMessage(&proxyman.SenderConfig{StreamSettings: stream}),
			ProxySettings: serial.ToTypedMessage(&vlessout.Config{Vnext: &protocol.ServerEndpoint{
				Address: xnet.NewIPOrDomain(xnet.ParseAddress(l.Host)),
				Port:    uint32(l.Port),
				User:    &protocol.User{Account: serial.ToTypedMessage(account)},
			}}),
		}},
	}

	inst, err := core.New(conf)
	if err != nil {
		return nil, fmt.Errorf("relay: %w", err)
	}
	if err := inst.Start(); err != nil {
		inst.Close()
		return nil, fmt.Errorf("relay: %w", err)
	}
	return &Dialer{Link: l, core: inst}, nil
}

func span(s vlesslink.Span) *splithttp.RangeConfig {
	return &splithttp.RangeConfig{From: s.From, To: s.To}
}

func xhttpOf(x *vlesslink.XHTTP) *splithttp.Config {
	return &splithttp.Config{
		Host:                 x.Host,
		Path:                 x.Path,
		Mode:                 x.Mode,
		Headers:              x.Headers,
		XPaddingBytes:        span(x.XPaddingBytes),
		XPaddingObfsMode:     x.XPaddingObfsMode,
		XPaddingKey:          x.XPaddingKey,
		XPaddingHeader:       x.XPaddingHeader,
		XPaddingPlacement:    x.XPaddingPlacement,
		XPaddingMethod:       x.XPaddingMethod,
		UplinkHTTPMethod:     x.UplinkHTTPMethod,
		SessionPlacement:     x.SessionPlacement,
		SessionKey:           x.SessionKey,
		SeqPlacement:         x.SeqPlacement,
		SeqKey:               x.SeqKey,
		UplinkDataPlacement:  x.UplinkDataPlacement,
		UplinkDataKey:        x.UplinkDataKey,
		UplinkChunkSize:      span(x.UplinkChunkSize),
		NoGRPCHeader:         x.NoGRPCHeader,
		NoSSEHeader:          x.NoSSEHeader,
		ScMaxEachPostBytes:   span(x.ScMaxEachPostBytes),
		ScMinPostsIntervalMs: span(x.ScMinPostsIntervalMs),
		ScMaxBufferedPosts:   int64(x.ScMaxBufferedPosts),
		ScStreamUpServerSecs: span(x.ScStreamUpServerSecs),
		ServerMaxHeaderBytes: int32(x.ServerMaxHeaderBytes),
		Xmux: &splithttp.XmuxConfig{
			MaxConcurrency:   span(x.Xmux.MaxConcurrency),
			MaxConnections:   span(x.Xmux.MaxConnections),
			CMaxReuseTimes:   span(x.Xmux.CMaxReuseTimes),
			HMaxRequestTimes: span(x.Xmux.HMaxRequestTimes),
			HMaxReusableSecs: span(x.Xmux.HMaxReusableSecs),
			HKeepAlivePeriod: int64(x.Xmux.HKeepAlivePeriod),
		},
	}
}

func secure(stream *internet.StreamConfig, settings proto.Message) {
	held := serial.ToTypedMessage(settings)
	stream.SecurityType = held.Type
	stream.SecuritySettings = []*serial.TypedMessage{held}
}

func (d *Dialer) Dial(ctx context.Context, endpoint string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, err
	}
	p, err := xnet.PortFromString(port)
	if err != nil {
		return nil, err
	}
	session, cancel := context.WithCancel(context.WithoutCancel(ctx))
	conn, err := core.Dial(session, d.core, xnet.TCPDestination(xnet.ParseAddress(host), p))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("relay %s: %w", d.Link.Server(), err)
	}
	return &carried{Conn: conn, cancel: cancel}, nil
}

func (d *Dialer) Close() error { return d.core.Close() }

type carried struct {
	net.Conn
	cancel context.CancelFunc
}

func (c *carried) Close() error {
	err := c.Conn.Close()
	c.cancel()
	return err
}

var errDropped = errors.New("the relay dropped the connection")

func dropped(err error) error {
	if errors.Is(err, io.ErrClosedPipe) {
		return errDropped
	}
	return err
}

func (c *carried) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	return n, dropped(err)
}

func (c *carried) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	return n, dropped(err)
}
