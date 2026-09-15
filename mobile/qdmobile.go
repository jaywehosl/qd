package qdmobile

import (
	"context"
	"encoding/hex"
	"net/netip"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jaywehosl/quic-diver/internal/adblock"
	"github.com/jaywehosl/quic-diver/internal/clientapi"
	"github.com/jaywehosl/quic-diver/internal/clientdns"
	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/qcli"
	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
	"github.com/jaywehosl/quic-diver/internal/qsrv/uplink/relay"
	"github.com/jaywehosl/quic-diver/internal/qwire"
)

const (
	localIP     = "10.7.0.2"
	localPrefix = 24
	dnsIP       = "10.7.0.53"
	safeMTU     = 1392

	readers = 1
)

type Protector interface {
	Protect(fd int) bool
	Lookup(host string) string
}

type Host interface {
	Establish(plan string) int
	Teardown()
	Note(text string)
	Owner(proto int, source string, sourcePort int, target string, targetPort int) string
}

type Client struct {
	mu sync.Mutex

	db     *clientstate.DB
	seen   *clientapi.Visits
	api    *clientapi.API
	host   Host
	device clientapi.Device

	protector Protector
	talk      *qwire.Dialer
	marks     *marks
	key       *qdcrypt.Key
	session   uint32
	mtu       int
	rate      atomic.Int64
	netTag    string
	quit      chan struct{}

	running  bool
	stop     chan struct{}
	live     *qcli.Tunnel
	liveStop context.CancelFunc
	dialing  context.CancelFunc
	src      packet.Source
	turn     sync.Mutex
	server   string
	appSplit string
	dns      *clientdns.Resolver
	gone     chan struct{}
}

func Open(stateDir string, host Host, protector Protector, deviceID, model, name string) (*Client, error) {
	markJournal(stateDir)
	say("open: state=%s", stateDir)

	db, err := clientstate.Open(filepath.Join(stateDir, "client.db"))
	if err != nil {
		return nil, err
	}

	settings, err := db.Settings()
	if err != nil {
		db.Close()
		return nil, err
	}

	c := &Client{
		db:        db,
		host:      host,
		protector: protector,
		mtu:       safeMTU,
		quit:      make(chan struct{}),
		device: clientapi.Device{
			ID: deviceID, Platform: "android", Model: model, Kind: "phone", Name: name,
		},
	}
	c.rate.Store(int64(settings.FixedRate))
	c.seen = clientapi.NewVisits(db, adblock.Default(), settings.Adblock)
	c.marks = newMarks(host)
	c.marks.reload(db)

	if raw, err := hex.DecodeString(settings.NetworkKey); err == nil && len(raw) == qdcrypt.KeySize {
		var k qdcrypt.Key
		copy(k[:], raw)
		c.key = &k
	}

	c.api = clientapi.New(db, platform{c: c}, c.seen, c.key)
	if protector != nil {
		relay.Lookup = func(host string) []netip.Addr { return lookupOn(protector, host) }
	}
	c.upkeep()
	return c, nil
}

func lookupOn(protector Protector, host string) []netip.Addr {
	var out []netip.Addr
	for _, text := range strings.Split(protector.Lookup(host), ",") {
		if addr, err := netip.ParseAddr(text); err == nil {
			out = append(out, addr.Unmap())
		}
	}
	return out
}

func (c *Client) Close() error {
	close(c.quit)
	c.Disconnect()
	c.seen.Close()
	return c.db.Close()
}

func (c *Client) Import(uri string) error {
	if err := c.api.Import(uri); err != nil {
		say("import: %v", err)
		return err
	}
	say("import: link taken")

	settings, err := c.db.Settings()
	if err != nil {
		return err
	}
	if raw, err := hex.DecodeString(settings.NetworkKey); err == nil && len(raw) == qdcrypt.KeySize {
		var k qdcrypt.Key
		copy(k[:], raw)
		c.mu.Lock()
		c.key = &k
		c.mu.Unlock()
		c.tellWire()
	}
	return nil
}

func (c *Client) Imported() bool {
	sub, err := c.db.Subscription()
	return err == nil && sub.Imported
}

func (c *Client) Connect() error    { return c.api.Connect() }
func (c *Client) Disconnect() error { return c.api.Disconnect() }

func (c *Client) StateJSON() string {
	blob, err := c.api.StateJSON()
	if err != nil {
		return "{}"
	}
	return blob
}

func (c *Client) SettingsJSON() string {
	blob, err := c.api.SettingsJSON()
	if err != nil {
		return "{}"
	}
	return blob
}

func (c *Client) MarkNoticeRead(id int) error { return c.api.MarkNoticeRead(id) }

func (c *Client) AboutJSON() string {
	blob, err := c.api.AboutJSON()
	if err != nil {
		return "{}"
	}
	return blob
}

func (c *Client) NodesJSON() string {
	blob, err := c.api.NodesJSON()
	if err != nil {
		return "[]"
	}
	return blob
}

func (c *Client) RulesJSON() string {
	blob, err := c.api.RulesJSON()
	if err != nil {
		return "{}"
	}
	return blob
}

func (c *Client) SaveRulesJSON(raw string) error    { return c.api.SaveRulesJSON(raw) }
func (c *Client) SaveSettingsJSON(raw string) error { return c.api.SaveSettingsJSON(raw) }
func (c *Client) Reset(subscription bool) error     { return c.api.Reset(subscription) }
func (c *Client) SetEgress(on bool) error           { return c.api.SetEgress(on) }
func (c *Client) SetAdblock(on bool) error          { return c.api.SetAdblock(on) }

func (c *Client) Refresh() error {
	_, err := c.api.Refresh()
	return err
}

func (c *Client) Label() string {
	sub, err := c.db.Subscription()
	if err != nil {
		return ""
	}
	if sub.Label != "" {
		return sub.Label
	}
	return sub.Tag
}

func (c *Client) Node() string {
	node, ok := c.api.Selected()
	if !ok {
		return ""
	}
	return node.Name
}

func (c *Client) Ping() int {
	c.mu.Lock()
	warm := c.dns
	c.mu.Unlock()

	if warm != nil {
		if ms := warm.RTT(); ms >= 0 {
			return ms
		}
	}

	return -1
}
