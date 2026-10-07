//go:build linux

package main

import (
	"bufio"
	"encoding/binary"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/jaywehosl/qd/internal/clientstate"
)

const (
	tableTTL      = 5 * time.Second
	missCooldown  = 40 * time.Millisecond
	ownerStep     = 2 * time.Millisecond
	ownerPatience = 60 * time.Millisecond
)

type platformRouter struct {
	tbl     atomic.Pointer[map[portKey]uint32]
	refresh chan struct{}
}

func (r *procRouter) start() {
	r.plat.refresh = make(chan struct{}, 1)
	go r.keepTable()
}

func (r *procRouter) flush() {}

func (r *procRouter) pidOf(key portKey, _ netip.Addr) (pid uint32, found, read bool) {
	other := key
	other.v6 = !key.v6
	for waited := time.Duration(0); ; waited += ownerStep {
		if held := r.plat.tbl.Load(); held != nil {
			if pid, ok := (*held)[key]; ok {
				return pid, true, true
			}
			if pid, ok := (*held)[other]; ok {
				return pid, true, true
			}
		}
		if waited >= ownerPatience {
			return 0, false, true
		}
		select {
		case r.plat.refresh <- struct{}{}:
		default:
		}
		time.Sleep(ownerStep)
	}
}

func (r *procRouter) keepTable() {
	tick := time.NewTicker(tableTTL)
	defer tick.Stop()
	for {
		if r.fixed.Load() == nil {
			next := make(map[portKey]uint32, 512)
			readSockets(next)
			if len(next) > 0 {
				r.plat.tbl.Store(&next)
			}
		}
		select {
		case <-tick.C:
		case <-r.plat.refresh:
			time.Sleep(missCooldown)
		}
	}
}

func readSockets(into map[portKey]uint32) {
	inodes := map[uint64]portKey{}
	for _, t := range []struct {
		file  string
		proto uint8
		v6    bool
	}{
		{"/proc/net/tcp", protoTCP, false}, {"/proc/net/tcp6", protoTCP, true},
		{"/proc/net/udp", protoUDP, false}, {"/proc/net/udp6", protoUDP, true},
	} {
		readNetTable(t.file, t.proto, t.v6, inodes)
	}
	if len(inodes) == 0 {
		return
	}
	eachSocket(func(pid uint32, inode uint64) {
		if key, ok := inodes[inode]; ok {
			into[key] = pid
		}
	})
}

func readNetTable(file string, proto uint8, v6 bool, into map[uint64]portKey) {
	f, err := os.Open(file)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Scan()
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 10 {
			continue
		}
		colon := strings.LastIndexByte(fields[1], ':')
		if colon < 0 {
			continue
		}
		port, err := strconv.ParseUint(fields[1][colon+1:], 16, 16)
		if err != nil {
			continue
		}
		inode, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil || inode == 0 {
			continue
		}
		into[inode] = portKey{proto: proto, v6: v6, port: uint16(port)}
	}
}

func eachSocket(fn func(pid uint32, inode uint64)) {
	for _, name := range dirNames("/proc") {
		pid, err := strconv.ParseUint(name, 10, 32)
		if err != nil {
			continue
		}
		dir := "/proc/" + name + "/fd/"
		for _, fd := range dirNames(dir) {
			link, err := os.Readlink(dir + fd)
			if err != nil || !strings.HasPrefix(link, "socket:[") {
				continue
			}
			if inode, err := strconv.ParseUint(link[8:len(link)-1], 10, 64); err == nil {
				fn(uint32(pid), inode)
			}
		}
	}
}

func dirNames(dir string) []string {
	f, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer f.Close()
	names, _ := f.Readdirnames(-1)
	return names
}

func lookupProcess(pid uint32) procIdent {
	path, err := os.Readlink("/proc/" + strconv.FormatUint(uint64(pid), 10) + "/exe")
	if err != nil || path == "" {
		return procIdent{}
	}
	path = strings.TrimSuffix(path, " (deleted)")
	return procIdent{name: filepath.Base(path), path: path}
}

func (r *procRouter) dropRerouted() int {
	r.mu.RLock()
	oldPath, oldName, oldDef := r.prevByPath, r.prevByName, r.prevDef
	newPath, newName, newDef := r.byPath, r.byName, r.def
	r.mu.RUnlock()

	if oldPath == nil && oldName == nil {
		return 0
	}
	identFor := identMemo()
	return dropTunnelled(func(pid uint32) bool {
		ident, ok := identFor(pid)
		return ok && roleIn(ident, oldPath, oldName, oldDef) != roleIn(ident, newPath, newName, newDef)
	})
}

func (r *procRouter) dropInherited() int {
	return r.dropRoles(func(role string) bool { return role == clientstate.RoleTunnel })
}

func (r *procRouter) dropCarried() int {
	return r.dropRoles(func(role string) bool { return role != clientstate.RoleDirect })
}

func (r *procRouter) dropRoles(pick func(role string) bool) int {
	byPath, byName, def := map[string]string{}, map[string]string{}, clientstate.RoleTunnel
	if r != nil {
		r.mu.RLock()
		byPath, byName, def = r.byPath, r.byName, r.def
		r.mu.RUnlock()
	}
	identFor := identMemo()
	return dropTunnelled(func(pid uint32) bool {
		role := def
		if ident, ok := identFor(pid); ok {
			role = roleIn(ident, byPath, byName, def)
		}
		return pick(role)
	})
}

const liveTCP = 1<<1 | 1<<2 | 1<<3 | 1<<8

type diagSock struct {
	id    [48]byte
	inode uint32
}

func dropTunnelled(pick func(pid uint32) bool) int {
	split.mu.Lock()
	up, addr := split.up, split.addr
	split.mu.Unlock()
	if !up || !addr.Is4() {
		return 0
	}

	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.NETLINK_SOCK_DIAG)
	if err != nil {
		return 0
	}
	defer unix.Close(fd)

	local := addr.As4()
	wanted := map[uint64]diagSock{}
	for _, s := range tcpSockets(fd) {
		if [4]byte(s.id[4:8]) == local {
			wanted[uint64(s.inode)] = s
		}
	}
	if len(wanted) == 0 {
		return 0
	}

	self := uint32(os.Getpid())
	dropped := 0
	eachSocket(func(pid uint32, inode uint64) {
		s, ok := wanted[inode]
		if !ok || pid == self {
			return
		}
		delete(wanted, inode)
		if pick(pid) && destroySock(fd, s.id) {
			dropped++
		}
	})
	return dropped
}

func diagRequest(kind, flags uint16, id []byte) []byte {
	b := make([]byte, unix.NLMSG_HDRLEN+56)
	binary.NativeEndian.PutUint32(b[0:], uint32(len(b)))
	binary.NativeEndian.PutUint16(b[4:], kind)
	binary.NativeEndian.PutUint16(b[6:], flags)
	b[16] = unix.AF_INET
	b[17] = unix.IPPROTO_TCP
	binary.NativeEndian.PutUint32(b[20:], liveTCP)
	copy(b[24:], id)
	return b
}

func tcpSockets(fd int) []diagSock {
	req := diagRequest(unix.SOCK_DIAG_BY_FAMILY, unix.NLM_F_REQUEST|unix.NLM_F_DUMP, nil)
	if unix.Sendto(fd, req, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}) != nil {
		return nil
	}
	var out []diagSock
	buf := make([]byte, 1<<16)
	for {
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if err != nil {
			return out
		}
		msgs, err := syscall.ParseNetlinkMessage(buf[:n])
		if err != nil {
			return out
		}
		for _, m := range msgs {
			if m.Header.Type == unix.NLMSG_DONE || m.Header.Type == unix.NLMSG_ERROR {
				return out
			}
			if len(m.Data) < 72 {
				continue
			}
			var s diagSock
			copy(s.id[:], m.Data[4:52])
			s.inode = binary.NativeEndian.Uint32(m.Data[68:72])
			out = append(out, s)
		}
	}
}

func destroySock(fd int, id [48]byte) bool {
	req := diagRequest(unix.SOCK_DESTROY, unix.NLM_F_REQUEST|unix.NLM_F_ACK, id[:])
	if unix.Sendto(fd, req, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}) != nil {
		return false
	}
	buf := make([]byte, 4096)
	n, _, err := unix.Recvfrom(fd, buf, 0)
	if err != nil {
		return false
	}
	msgs, err := syscall.ParseNetlinkMessage(buf[:n])
	if err != nil || len(msgs) == 0 || msgs[0].Header.Type != unix.NLMSG_ERROR || len(msgs[0].Data) < 4 {
		return false
	}
	return int32(binary.NativeEndian.Uint32(msgs[0].Data[:4])) == 0
}
