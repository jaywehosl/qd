//go:build windows || linux

package main

import (
	"cmp"
	"fmt"
	"net/netip"
	"sync/atomic"

	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/domainroute"
	"github.com/jaywehosl/qd/internal/qcli"
	"github.com/jaywehosl/qd/internal/qsrv"
)

var (
	liveTunnel atomic.Pointer[*qcli.Tunnel]
	exitTag    atomic.Pointer[string]
)

const anyExit = qsrv.AnyExit

const (
	protoTCP = 6
	protoUDP = 17
)

func setExit(egress bool) {
	tag := ""
	if egress {
		tag = anyExit
	}
	exitTag.Store(&tag)

	held := liveTunnel.Load()
	if held != nil {
		(*held).SetRoute(tag)
	}

	if n := routeByProcess.Load().dropInherited(); n > 0 {
		fmt.Printf("route    %d connections dropped so the new exit takes hold now\n", n)
	}
	if held != nil {
		(*held).Reroute()
	}
}

func routeTag() string {
	if held := exitTag.Load(); held != nil {
		return *held
	}
	return ""
}

var routeByDomain = domainroute.New()

func roleFor(src, dst netip.AddrPort, udp bool) string {
	def := clientstate.RoleTunnel
	if r := routeByProcess.Load(); r != nil {
		proto := uint8(protoTCP)
		if udp {
			proto = protoUDP
		}
		if role, own := r.OwnRoleForFlow(proto, src); own {
			return role
		}
		def = r.fallback()
	}
	return chained("", false, dst.Addr(), def)
}

func detours(src, dst netip.AddrPort, udp bool) bool {
	return roleFor(src, dst, udp) == clientstate.RoleDirect
}

func exitOfRole(role string) string {
	switch role {
	case clientstate.RoleEgress:
		return anyExit
	case clientstate.RoleNoEgress:
		return ""
	}
	return routeTag()
}

func (p hostPlatform) DomainFlows() map[string]int {
	out := map[string]int{}
	for _, addr := range openRemotes() {
		if rule, known := routeByDomain.RuleOf(addr); known {
			out[rule]++
		}
	}
	return out
}

func namedDirect(name string) bool {
	role, known := routeByDomain.RoleOfName(name)
	return known && role == clientstate.RoleDirect
}

func nameLeaves(name string) bool {
	role, _ := routeByDomain.RoleOfName(name)
	return exitOfRole(role) == anyExit
}

func exitFor(src, dst netip.AddrPort, udp bool) string {
	return exitOfRole(roleFor(src, dst, udp))
}

var (
	fixedRate  atomic.Int64
	bbrProfile atomic.Pointer[string]
)

func setCarriage(mbit int, profile string) {
	fixedRate.Store(int64(mbit))
	bbrProfile.Store(&profile)
}

func rateNow() int { return int(fixedRate.Load()) }

func profileNow() string {
	if held := bbrProfile.Load(); held != nil {
		return *held
	}
	return ""
}

func carriageNow() string {
	if rate := rateNow(); rate > 0 {
		return fmt.Sprintf("brutal %d", rate)
	}
	return "bbr " + cmp.Or(profileNow(), "standard")
}

func settingsCarriage(db *clientstate.DB) (int, string) {
	settings, err := db.Settings()
	if err != nil {
		return 0, ""
	}
	return settings.FixedRate, settings.BBRProfile
}
