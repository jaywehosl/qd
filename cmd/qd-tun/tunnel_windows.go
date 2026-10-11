//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/ippkt"
	"github.com/jaywehosl/qd/internal/qcli"
	"github.com/jaywehosl/qd/internal/qcli/guard"
	"github.com/jaywehosl/qd/internal/qcli/packet"
	"github.com/jaywehosl/qd/internal/qcli/windivert"

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
	if routeByDomain.Active() {
		if dst, ok := ippkt.Dst(pkt); ok {
			if role, known := routeByDomain.RoleOf(dst); known {
				return role == clientstate.RoleDirect
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
		held := t.bypass(live, keepOut)
		keptOut.Store(&held)
		filter := windivert.BuildFilter(windivert.CaptureConfig{
			TCP: true, UDP: true, DNS: t.servesDNS(),
			Bypass: t.bypass(live, keepOut),
		})
		src, err := openDriver(dll, filter)
		if err != nil {
			return nil, err
		}
		if r := routeByProcess.Load(); r != nil {
			go r.watchSockets(ctx, dll)
		}
		return src, nil
	}, nil
}

const (
	driverWait  = 15 * time.Second
	driverTries = 3
	driverPause = 700 * time.Millisecond
)

func openDriver(dll, filter string) (*windivert.Source, error) {
	type opened struct {
		src *windivert.Source
		err error
	}
	done := make(chan opened, 1)
	go func() {
		var got opened
		for try := 0; try < driverTries; try++ {
			if stale := windivert.Mend(dll); stale != "" {
				fmt.Printf("windivert a stale driver service pointing at %s was removed\n", stale)
			}
			if got.src, got.err = windivert.Open(dll, filter, 0); got.err == nil {
				break
			}
			fmt.Printf("windivert attempt %d of %d: %v; %s\n", try+1, driverTries, got.err, windivert.Describe(dll))
			time.Sleep(driverPause)
		}
		done <- got
	}()

	select {
	case got := <-done:
		if got.err != nil {
			return nil, driverError(got.err)
		}
		return got.src, nil
	case <-time.After(driverWait):
		go func() {
			if got := <-done; got.src != nil {
				got.src.Close()
			}
		}()
		fmt.Printf("windivert the driver did not answer in %s; %s\n", driverWait, windivert.Describe(dll))
		return nil, fmt.Errorf("windivert: the capture driver did not answer in %s, restart Windows to reset it", driverWait)
	}
}

func driverError(err error) error {
	switch {
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return fmt.Errorf("windivert: %w (run qd as administrator)", err)
	case errors.Is(err, windows.ERROR_FILE_NOT_FOUND), errors.Is(err, windows.ERROR_PATH_NOT_FOUND):
		return fmt.Errorf("windivert: %w (the capture driver is missing: an antivirus may have removed WinDivert64.sys, or its service is broken until Windows restarts)", err)
	case errors.Is(err, windows.ERROR_INVALID_IMAGE_HASH), errors.Is(err, windows.ERROR_DRIVER_BLOCKED):
		return fmt.Errorf("windivert: %w (Windows refused to load the capture driver)", err)
	}
	return fmt.Errorf("windivert: %w", err)
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
	if old := windivert.LegacyDir(); old != "" {
		os.Remove(filepath.Join(old, "WinDivert.dll"))
		os.Remove(filepath.Join(old, "WinDivert64.sys"))
	}
	return dll, nil
}

func releaseDriver() {
	dir, err := windivert.DefaultDir()
	if err != nil {
		return
	}
	if windivert.Unload(filepath.Join(dir, "WinDivert.dll"), filepath.Join(windivert.LegacyDir(), "WinDivert.dll")) {
		fmt.Printf("windivert the capture driver was asked to unload\n")
	}
}

func (p hostPlatform) RoutesByDomain() {}

var outside func(ctx context.Context, network string, dst netip.AddrPort) (net.Conn, error)
