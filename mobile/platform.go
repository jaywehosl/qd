package qdmobile

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/jaywehosl/qd/internal/clientapi"
	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/qdcrypt"
	"github.com/jaywehosl/qd/internal/qsrv/uplink/relay"
	"github.com/jaywehosl/qd/internal/update"
)

type platform struct {
	c *Client
}

func (p platform) Running() bool { return p.c.Running() }

func (p platform) Start(servers []string, relays []relay.Link, session uint32) error {
	if len(servers) == 0 {
		return fmt.Errorf("no entrypoint to dial")
	}
	return p.c.carry(servers, relays, session)
}

func (p platform) Stop() error {
	p.c.wanted.Store(false)
	p.c.stopCarry()
	if p.c.host != nil {
		p.c.host.Teardown()
	}
	return nil
}

func (p platform) SetKey(key *qdcrypt.Key) {
	p.c.mu.Lock()
	p.c.key = key
	p.c.mu.Unlock()
	p.c.tellWire()
}

func (p platform) SetExit(egress bool) { p.c.applyExit(egress) }

func (p platform) SetCarriage(mbit int, profile string) {
	p.c.rate.Store(int64(mbit))
	p.c.profile.Store(&profile)
	p.c.mu.Lock()
	held, running := p.c.carried, p.c.running
	p.c.mu.Unlock()
	if running && held != p.c.carriage() {
		say("carry: the congestion control changed, the tunnel comes back with it")
		go p.c.lost()
	}
}

func (c *Client) bbr() string {
	if held := c.profile.Load(); held != nil {
		return *held
	}
	return ""
}

func (c *Client) carriage() string {
	if rate := c.rate.Load(); rate > 0 {
		return fmt.Sprintf("brutal %d", rate)
	}
	return "bbr " + cmp.Or(c.bbr(), "standard")
}

func (p platform) SyncControlRelays(relays []relay.Link) { p.c.wire().SetRelays(relays) }

func (p platform) ServerName() string {
	p.c.mu.Lock()
	defer p.c.mu.Unlock()
	return p.c.server
}

func (p platform) Identify() clientapi.Device { return p.c.device }

func (p platform) Processes() []clientapi.Process { return []clientapi.Process{} }

func (p platform) RulesChanged() {
	if p.c.marks != nil {
		p.c.marks.reload(p.c.db)
	}
	p.c.reroute()
	p.c.restack()
}

func (c *Client) split() string {
	direct, allowed, carveOut := c.appLists()
	return fmt.Sprintf("%v|%v|%v", direct, allowed, carveOut)
}

func (c *Client) restack() {
	fresh := c.split()

	c.mu.Lock()
	same := fresh == c.appSplit
	c.appSplit = fresh
	running := c.running
	c.mu.Unlock()

	if same || !running {
		return
	}

	say("rules: direct list changed, rebuilding the tunnel")
	go c.relink()
}

func (c *Client) peerAddresses() []string {
	nodes, err := c.db.Nodes()
	if err != nil {
		return nil
	}

	seen := map[string]bool{}
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n.Address == "" || seen[n.Address] {
			continue
		}
		seen[n.Address] = true
		out = append(out, n.Address)
	}

	for _, address := range c.api.Peers() {
		if address == "" || seen[address] {
			continue
		}
		seen[address] = true
		out = append(out, address)
	}
	return out
}

func (c *Client) appLists() (direct []string, allowed []string, carveOut bool) {
	fallback, rules, err := c.db.RulesInForce()
	if err != nil {
		return nil, nil, false
	}

	for _, rule := range rules {
		name := strings.TrimSpace(rule.Process)
		if name == "" {
			continue
		}
		if rule.Role == clientstate.RoleDirect {
			direct = append(direct, name)
			continue
		}
		allowed = append(allowed, name)
	}

	if fallback == clientstate.RoleDirect {
		return nil, allowed, true
	}
	return direct, nil, false
}

func (p platform) HoldAutostart(on bool) error { return nil }

func (c *Client) hold(assigned, six netip.Prefix, mtu int) (int, error) {
	direct, allowed, carveOut := c.appLists()
	if carveOut {
		say("rules: only these apps enter the tunnel: %v", allowed)
	} else {
		say("rules: these apps bypass the tunnel: %v", direct)
	}

	c.mu.Lock()
	c.appSplit = fmt.Sprintf("%v|%v|%v", direct, allowed, carveOut)
	c.mu.Unlock()

	shape := map[string]any{
		"localIp":  assigned.Addr().String(),
		"prefix":   assigned.Bits(),
		"dns":      dnsIP,
		"mtu":      mtu,
		"exclude":  direct,
		"include":  allowed,
		"carveOut": carveOut,
		"peers":    c.peerAddresses(),
	}
	if six.IsValid() {
		shape["localIp6"] = six.Addr().String()
		shape["prefix6"] = six.Bits()
	}
	plan, err := json.Marshal(shape)
	if err != nil {
		return 0, err
	}

	fd := c.host.Establish(string(plan))
	if fd <= 0 {
		return 0, errors.New("the system refused to establish the tunnel")
	}
	return fd, nil
}

func (p platform) Install(tag string, open update.Opener, tick func(done, total int64)) error {
	path, err := update.Take(open, tag, "qd-android-arm64.apk", filepath.Join(p.c.dir, "update"), tick)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	say("update: %s fetched and verified, handing it to the package installer", tag)
	if !p.c.host.Install(path) {
		return errors.New("the package installer did not take the update")
	}
	return nil
}
