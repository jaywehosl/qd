//go:build windows

package main

import (
	"context"
	"fmt"
	"net/netip"
	"sync/atomic"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/ippkt"
	"github.com/jaywehosl/quic-diver/internal/qcli"
	"github.com/jaywehosl/quic-diver/internal/qcli/guard"
	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
	"github.com/jaywehosl/quic-diver/internal/qcli/windivert"

	"golang.org/x/sys/windows"
)

var keepSocket func(fd uintptr)

var lateAside atomic.Pointer[[]netip.Prefix]

func keepAsideReset() { lateAside.Store(nil) }

var dnsFlush = windows.NewLazySystemDLL("dnsapi.dll").NewProc("DnsFlushResolverCache")

func flushSystemDNS() { dnsFlush.Call() }

func keepAside(fresh []netip.Prefix) {
	next := append([]netip.Prefix{}, fresh...)
	if held := lateAside.Load(); held != nil {
		next = append(next, *held...)
	}
	lateAside.Store(&next)
}

func goesDirect(pkt []byte) bool {
	if late := lateAside.Load(); late != nil {
		if dst, ok := ippkt.Dst(pkt); ok {
			for _, p := range *late {
				if p.Contains(dst) {
					return true
				}
			}
		}
	}
	r := routeByProcess.Load()
	if r == nil || !r.Active() {
		return false
	}
	return r.RoleFor(pkt) == clientstate.RoleDirect
}

func (t *tunnel) opener() (sourceOpener, error) {
	dll, err := unpackDriver()
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, live *qcli.Tunnel, keepOut []netip.Prefix) (packet.Source, error) {
		filter := windivert.BuildFilter(windivert.CaptureConfig{
			TCP: true, UDP: true, DNS: t.servesDNS(),
			Bypass: t.bypass(live, keepOut),
		})
		src, err := windivert.Open(dll, filter, 0)
		if err != nil {
			return nil, fmt.Errorf("windivert: %w (run as administrator)", err)
		}
		if r := routeByProcess.Load(); r != nil {
			go r.watchSockets(ctx, dll)
		}
		return src, nil
	}, nil
}

func (t *tunnel) bypass(live *qcli.Tunnel, keepOut []netip.Prefix) []netip.Prefix {
	out := append([]netip.Prefix(nil), guard.New(nil).Bypasses()...)
	for _, p := range live.Peers() {
		out = append(out, netip.PrefixFrom(p, p.BitLen()))
	}
	for _, p := range live.RelayPeers() {
		out = append(out, netip.PrefixFrom(p, p.BitLen()))
	}
	return append(out, keepOut...)
}

func unpackDriver() (string, error) {
	dir, err := windivert.DefaultDir()
	if err != nil {
		return "", fmt.Errorf("driver folder: %w", err)
	}
	dll, err := windivert.Extract(dir)
	if err != nil {
		return "", fmt.Errorf("unpack windivert: %w", err)
	}
	return dll, nil
}
