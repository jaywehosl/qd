//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"

	"github.com/jaywehosl/quic-diver/internal/qcli"
	"github.com/jaywehosl/quic-diver/internal/qcli/guard"
	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
	"github.com/jaywehosl/quic-diver/internal/qcli/packet/tun"
)

const (
	tunName    = "qd0"
	routeTable = 5200
	socketMark = 0x5d71
	tunDNS     = "10.7.0.53"
)

const (
	ruleDNS = routeTable + iota
	ruleOwn
	ruleAside
	ruleLocal
	ruleTunnel
)

var tunIndex atomic.Int32

var keepSocket = func(fd uintptr) {
	if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_MARK, socketMark); err != nil {
		fmt.Printf("socket   fd %d stays unmarked, it may loop through the tunnel: %v\n", fd, err)
	}
}

func goesDirect(pkt []byte) bool { return false }

func (t *tunnel) opener() (sourceOpener, error) {
	if os.Geteuid() != 0 {
		return nil, errors.New("the tunnel needs root; run it as the qd-client service")
	}
	clearRules()
	restoreDNS()

	return func(ctx context.Context, live *qcli.Tunnel, keepOut []netip.Prefix) (packet.Source, error) {
		return raise(live, keepOut, t.cfg.MTU, t.servesDNS())
	}, nil
}

type heldTun struct {
	*tun.Source
	once sync.Once
	undo []func()
}

func (h *heldTun) Close() error {
	err := h.Source.Close()
	h.once.Do(func() {
		for i := len(h.undo) - 1; i >= 0; i-- {
			h.undo[i]()
		}
		tunIndex.Store(0)
		fmt.Printf("tun      %s down, rules and dns given back\n", tunName)
	})
	return err
}

func raise(live *qcli.Tunnel, keepOut []netip.Prefix, mtu int, dns bool) (packet.Source, error) {
	assigned := live.Assigned()
	if len(assigned) == 0 {
		return nil, errors.New("the node assigned no address")
	}

	fd, name, err := createTun(tunName)
	if err != nil {
		return nil, err
	}
	src, err := tun.Open(fd)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	held := &heldTun{Source: src, undo: []func(){clearRules}}
	fail := func(err error) (packet.Source, error) {
		held.Close()
		return nil, err
	}

	if err := ip("link", "set", name, "mtu", strconv.Itoa(mtu), "up"); err != nil {
		return fail(err)
	}
	if err := ip("addr", "add", assigned[0].String(), "dev", name); err != nil {
		return fail(err)
	}
	if nic, err := net.InterfaceByName(name); err == nil {
		tunIndex.Store(int32(nic.Index))
	}

	table := strconv.Itoa(routeTable)
	if err := ip("route", "replace", "default", "dev", name, "table", table); err != nil {
		return fail(err)
	}
	v6 := ip("-6", "route", "replace", "default", "dev", name, "table", table) == nil
	if !v6 {
		fmt.Printf("tun      no ipv6 on this machine, only v4 rides the tunnel\n")
	}

	var aside []netip.Prefix
	seen := map[netip.Prefix]bool{}
	for _, p := range append(append([]netip.Prefix{}, guard.New(nil).Bypasses()...), keepOut...) {
		if p = p.Masked(); !seen[p] {
			seen[p] = true
			aside = append(aside, p)
		}
	}
	for _, family := range []string{"-4", "-6"} {
		if family == "-6" && !v6 {
			continue
		}
		var steps [][]string
		if family == "-4" && dns {
			steps = append(steps, []string{"to", tunDNS + "/32", "lookup", table, "priority", strconv.Itoa(ruleDNS)})
		}
		steps = append(steps, []string{"fwmark", strconv.Itoa(socketMark), "lookup", "main", "priority", strconv.Itoa(ruleOwn)})
		for _, p := range aside {
			if (family == "-4") == p.Addr().Is4() {
				steps = append(steps, []string{"to", p.String(), "lookup", "main", "priority", strconv.Itoa(ruleAside)})
			}
		}
		steps = append(steps,
			[]string{"lookup", "main", "suppress_prefixlength", "0", "priority", strconv.Itoa(ruleLocal)},
			[]string{"lookup", table, "priority", strconv.Itoa(ruleTunnel)},
		)
		for _, s := range steps {
			if err := ip(append([]string{family, "rule", "add"}, s...)...); err != nil {
				return fail(err)
			}
		}
	}

	if dns {
		undo, err := pointDNS(name)
		if err != nil {
			return fail(fmt.Errorf("dns: %w", err))
		}
		held.undo = append(held.undo, undo)
	}

	splitUp(assigned[0].Addr())
	held.undo = append(held.undo, splitDown)

	fmt.Printf("tun      %s up with %s, mtu %d, %d prefixes kept aside\n", name, assigned[0], mtu, len(aside))
	return held, nil
}

func createTun(name string) (int, string, error) {
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, "", fmt.Errorf("open /dev/net/tun: %w", err)
	}
	ifr, err := unix.NewIfreq(name)
	if err != nil {
		unix.Close(fd)
		return -1, "", err
	}
	ifr.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI)
	if err := unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr); err != nil {
		unix.Close(fd)
		return -1, "", fmt.Errorf("create %s: %w", name, err)
	}
	return fd, ifr.Name(), nil
}

func clearRules() {
	for _, family := range []string{"-4", "-6"} {
		for prio := ruleDNS; prio <= ruleTunnel; prio++ {
			for range 256 {
				if _, err := run("ip", family, "rule", "del", "priority", strconv.Itoa(prio)); err != nil {
					break
				}
			}
		}
	}
}

func ip(args ...string) error {
	if out, err := run("ip", args...); err != nil {
		return fmt.Errorf("ip %v: %s", args, out)
	}
	return nil
}
