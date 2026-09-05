//go:build windows

package main

import (
	"fmt"
	"net/netip"
	"sync/atomic"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/qcli"
	"github.com/jaywehosl/quic-diver/internal/qsrv"
)

var (
	liveTunnel atomic.Pointer[*qcli.Tunnel]
	exitTag    atomic.Pointer[string]
)

const anyExit = qsrv.AnyExit

func setExit(egress bool) {
	tag := ""
	if egress {
		tag = anyExit
	}
	exitTag.Store(&tag)

	if held := liveTunnel.Load(); held != nil {
		(*held).SetRoute(tag)
	}

	if n := routeByProcess.Load().dropInherited(); n > 0 {
		fmt.Printf("route    %d connections dropped so the new exit takes hold now\n", n)
	}
}

func routeTag() string {
	if held := exitTag.Load(); held != nil {
		return *held
	}
	return ""
}

func exitFor(src, _ netip.AddrPort, udp bool) string {
	port := src.Port()

	r := routeByProcess.Load()
	if r == nil || !r.Active() {
		return routeTag()
	}
	proto := uint8(protoTCP)
	if udp {
		proto = protoUDP
	}
	switch r.RoleForPort(proto, port) {
	case clientstate.RoleEgress:
		return anyExit
	case clientstate.RoleNoEgress:
		return ""
	}
	return routeTag()
}

func goesDirect(pkt []byte) bool {
	r := routeByProcess.Load()
	if r == nil || !r.Active() {
		return false
	}
	return r.RoleFor(pkt) == clientstate.RoleDirect
}

var fixedRate atomic.Int64

func setFixedRate(mbit int) { fixedRate.Store(int64(mbit)) }

func rateNow() int { return int(fixedRate.Load()) }

func settingsFixedRate(db *clientstate.DB) int {
	settings, err := db.Settings()
	if err != nil {
		return 0
	}
	return settings.FixedRate
}
