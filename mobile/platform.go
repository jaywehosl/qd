package qdmobile

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/jaywehosl/quic-diver/internal/clientapi"
	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
)

type platform struct {
	c *Client
}

func (p platform) Running() bool { return p.c.Running() }

func (p platform) Start(servers []string, session uint32) error {
	if len(servers) == 0 {
		return fmt.Errorf("no entrypoint to dial")
	}
	return p.c.carry(servers, session)
}

func (p platform) Stop() error {
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

func (p platform) SetFixedRate(mbit int) {
	p.c.rate.Store(int64(mbit))
}

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
	rules, err := c.db.Rules()
	if err != nil {
		return nil, nil, false
	}
	fallback, err := c.db.DefaultRole()
	if err != nil {
		fallback = clientstate.RoleTunnel
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

func (p platform) AutostartHeld() bool { return false }

func (c *Client) hold(assigned netip.Prefix, mtu int) (int, error) {
	direct, allowed, carveOut := c.appLists()

	c.mu.Lock()
	c.appSplit = fmt.Sprintf("%v|%v|%v", direct, allowed, carveOut)
	c.mu.Unlock()

	plan, err := json.Marshal(map[string]any{
		"localIp":  assigned.Addr().String(),
		"prefix":   assigned.Bits(),
		"dns":      dnsIP,
		"mtu":      mtu,
		"exclude":  direct,
		"include":  allowed,
		"carveOut": carveOut,
		"peers":    c.peerAddresses(),
	})
	if err != nil {
		return 0, err
	}

	fd := c.host.Establish(string(plan))
	if fd <= 0 {
		return 0, errors.New("the system refused to establish the tunnel")
	}
	return fd, nil
}
