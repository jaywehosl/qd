//go:build linux

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
)

const (
	cgroupRoot   = "/sys/fs/cgroup"
	directGroup  = "qd-direct"
	tunnelGroup  = "qd-tunnel"
	sessionTree  = "/user.slice/"
	splitRescan  = 3 * time.Second
	procEventExe = 0x00000002
)

type splitMode int

const (
	splitNone splitMode = iota
	splitDirect
	splitTunnel
)

var split struct {
	mu      sync.Mutex
	mode    splitMode
	up      bool
	addr    netip.Addr
	started bool
	origin  map[uint32]string
	warned  map[string]bool
	broken  bool
}

func splitRules(r *procRouter) {
	r.mu.RLock()
	def := r.def
	direct := false
	for _, role := range r.byName {
		direct = direct || role == clientstate.RoleDirect
	}
	for _, role := range r.byPath {
		direct = direct || role == clientstate.RoleDirect
	}
	r.mu.RUnlock()

	mode := splitNone
	switch {
	case def == clientstate.RoleDirect:
		mode = splitTunnel
	case direct:
		mode = splitDirect
	}

	split.mu.Lock()
	was := split.mode
	split.mode = mode
	if split.origin == nil {
		split.origin, split.warned = map[uint32]string{}, map[string]bool{}
	}
	start := !split.started && mode != splitNone
	split.started = split.started || start
	split.mu.Unlock()

	if mode != splitNone && !readySplit(mode) {
		return
	}
	if was != mode && was != splitNone {
		evacuate(groupOf(was))
	}
	if start {
		go keepSplit()
	}
	sweep()
	renderSplit()
}

func readySplit(mode splitMode) bool {
	split.mu.Lock()
	defer split.mu.Unlock()
	if split.broken {
		return false
	}
	fail := func(why string) bool {
		fmt.Printf("routing  direct rules stay in the tunnel: %s\n", why)
		split.broken = true
		return false
	}
	var fs unix.Statfs_t
	if unix.Statfs(cgroupRoot, &fs) != nil || fs.Type != unix.CGROUP2_SUPER_MAGIC {
		return fail("this system has no unified cgroup v2 tree")
	}
	if _, err := exec.LookPath("nft"); err != nil {
		return fail("nft is missing, install nftables")
	}
	if err := os.MkdirAll(cgroupRoot+"/"+groupOf(mode), 0o755); err != nil {
		return fail(err.Error())
	}
	return true
}

func groupOf(mode splitMode) string {
	if mode == splitTunnel {
		return tunnelGroup
	}
	return directGroup
}

func splitUp(addr netip.Addr) {
	split.mu.Lock()
	split.up, split.addr = true, addr
	split.mu.Unlock()
	renderSplit()
}

func splitDown() {
	split.mu.Lock()
	split.up = false
	split.mu.Unlock()
	renderSplit()
}

func renderSplit() {
	split.mu.Lock()
	mode, up, addr, broken := split.mode, split.up, split.addr, split.broken
	split.mu.Unlock()

	script := "table inet qd\ndelete table inet qd\n"
	if up && mode != splitNone && !broken {
		match := `socket cgroupv2 level 1 "` + directGroup + `"`
		if mode == splitTunnel {
			match = `socket cgroupv2 level 1 != "` + tunnelGroup + `"`
		}
		mark := strconv.Itoa(socketMark)
		script += "table inet qd {\n" +
			"\tchain out {\n\t\ttype route hook output priority mangle; policy accept;\n" +
			"\t\t" + match + " meta mark set " + mark + " ct mark set " + mark + "\n\t}\n" +
			"\tchain pre {\n\t\ttype filter hook prerouting priority mangle; policy accept;\n" +
			"\t\tct mark " + mark + " meta mark set " + mark + "\n\t}\n" +
			"\tchain post {\n\t\ttype nat hook postrouting priority srcnat; policy accept;\n" +
			"\t\tmeta mark " + mark + ` oifname != { "` + tunName + `", "lo" } ip saddr ` + addr.String() + " masquerade\n\t}\n" +
			"}\n"
		os.WriteFile("/proc/sys/net/ipv4/conf/all/src_valid_mark", []byte("1"), 0o644)
	}

	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(script)
	if out, err := cmd.CombinedOutput(); err != nil && !errors.Is(err, exec.ErrNotFound) {
		fmt.Printf("routing  nft: %v %s\n", err, strings.TrimSpace(string(out)))
	}
}

func keepSplit() {
	events := make(chan uint32, 256)
	go func() {
		if err := watchExecs(events); err != nil {
			fmt.Printf("routing  no exec events, new programs are caught within %s: %v\n", splitRescan, err)
		}
	}()
	tick := time.NewTicker(splitRescan)
	defer tick.Stop()
	for {
		select {
		case pid := <-events:
			if r := routeByProcess.Load(); r != nil {
				r.forgetPid(pid)
			}
			place(pid)
		case <-tick.C:
			sweep()
		}
	}
}

func sweep() {
	for _, name := range dirNames("/proc") {
		if pid, err := strconv.ParseUint(name, 10, 32); err == nil {
			place(uint32(pid))
		}
	}
	split.mu.Lock()
	for pid := range split.origin {
		if _, err := os.Stat("/proc/" + strconv.FormatUint(uint64(pid), 10)); err != nil {
			delete(split.origin, pid)
		}
	}
	split.mu.Unlock()
}

func place(pid uint32) {
	split.mu.Lock()
	mode, broken := split.mode, split.broken
	split.mu.Unlock()
	if mode == splitNone || broken {
		return
	}
	r := routeByProcess.Load()
	if r == nil {
		return
	}
	ident := lookupProcess(pid)
	if ident.name == "" {
		return
	}
	role := r.roleNow(ident)
	group := groupOf(mode)
	want := (role == clientstate.RoleDirect) == (mode == splitDirect)
	now := cgroupOf(pid)
	inside := now == "/"+group

	switch {
	case want && !inside:
		if !strings.HasPrefix(now, sessionTree) {
			split.mu.Lock()
			if !split.warned[ident.name] {
				split.warned[ident.name] = true
				fmt.Printf("routing  %s runs outside a desktop session, its rule cannot be applied\n", ident.name)
			}
			split.mu.Unlock()
			return
		}
		if moveTo(pid, "/"+group) {
			split.mu.Lock()
			split.origin[pid] = now
			split.mu.Unlock()
		}
	case !want && inside:
		giveBack(pid)
	}
}

func evacuate(group string) {
	raw, err := os.ReadFile(cgroupRoot + "/" + group + "/cgroup.procs")
	if err != nil {
		return
	}
	for _, line := range strings.Fields(string(raw)) {
		if pid, err := strconv.ParseUint(line, 10, 32); err == nil {
			giveBack(uint32(pid))
		}
	}
}

func giveBack(pid uint32) {
	split.mu.Lock()
	back, known := split.origin[pid]
	delete(split.origin, pid)
	split.mu.Unlock()
	if !known || !moveTo(pid, back) {
		moveTo(pid, "/")
	}
}

func moveTo(pid uint32, group string) bool {
	path := cgroupRoot + strings.TrimSuffix(group, "/") + "/cgroup.procs"
	return os.WriteFile(path, []byte(strconv.FormatUint(uint64(pid), 10)), 0o644) == nil
}

func cgroupOf(pid uint32) string {
	raw, err := os.ReadFile("/proc/" + strconv.FormatUint(uint64(pid), 10) + "/cgroup")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if path, ok := strings.CutPrefix(line, "0::"); ok {
			return path
		}
	}
	return ""
}

func (r *procRouter) roleNow(ident procIdent) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.roleOf(ident, r.byPath, r.byName, r.def)
}

func (r *procRouter) forgetPid(pid uint32) {
	r.pidMu.Lock()
	delete(r.pids, pid)
	r.pidMu.Unlock()
}

func watchExecs(out chan<- uint32) error {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.NETLINK_CONNECTOR)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	const cnIdxProc, cnValProc, listen = 1, 1, 1
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK, Groups: cnIdxProc}); err != nil {
		return err
	}
	unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, 1<<20)

	msg := make([]byte, unix.NLMSG_HDRLEN+20+4)
	binary.NativeEndian.PutUint32(msg[0:], uint32(len(msg)))
	binary.NativeEndian.PutUint16(msg[4:], unix.NLMSG_DONE)
	binary.NativeEndian.PutUint32(msg[16:], cnIdxProc)
	binary.NativeEndian.PutUint32(msg[20:], cnValProc)
	binary.NativeEndian.PutUint16(msg[32:], 4)
	binary.NativeEndian.PutUint32(msg[36:], listen)
	if err := unix.Sendto(fd, msg, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return err
	}

	buf := make([]byte, 1<<16)
	for {
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if errors.Is(err, unix.ENOBUFS) {
			go sweep()
			continue
		}
		if err != nil {
			return err
		}
		msgs, err := syscall.ParseNetlinkMessage(buf[:n])
		if err != nil {
			continue
		}
		for _, m := range msgs {
			ev := m.Data
			if len(ev) < 20+24 {
				continue
			}
			ev = ev[20:]
			if binary.NativeEndian.Uint32(ev[0:4]) != procEventExe {
				continue
			}
			select {
			case out <- binary.NativeEndian.Uint32(ev[20:24]):
			default:
			}
		}
	}
}
