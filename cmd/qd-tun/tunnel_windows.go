//go:build windows

package main

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/qcli"
	"github.com/jaywehosl/quic-diver/internal/qcli/guard"
	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
	"github.com/jaywehosl/quic-diver/internal/qcli/windivert"
)

var keepSocket func(fd uintptr)

func goesDirect(pkt []byte) bool {
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
