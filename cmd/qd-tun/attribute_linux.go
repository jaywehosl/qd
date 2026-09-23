//go:build linux

package main

import (
	"bufio"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
)

func readSockets(into map[portKey]uint32) {
	inodes := map[uint64]portKey{}
	for _, t := range []struct {
		file  string
		proto uint8
	}{
		{"/proc/net/tcp", protoTCP}, {"/proc/net/tcp6", protoTCP},
		{"/proc/net/udp", protoUDP}, {"/proc/net/udp6", protoUDP},
	} {
		readNetTable(t.file, t.proto, inodes)
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

func readNetTable(file string, proto uint8, into map[uint64]portKey) {
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
		into[inode] = portKey{proto: proto, port: uint16(port)}
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
	return dropTunnelled(func(pid uint32) bool {
		ident, ok := r.identFor(pid)
		return ok && r.roleOf(ident, oldPath, oldName, oldDef) != r.roleOf(ident, newPath, newName, newDef)
	})
}

func (r *procRouter) dropInherited() int {
	byPath, byName, def := map[string]string{}, map[string]string{}, clientstate.RoleTunnel
	if r != nil {
		r.mu.RLock()
		byPath, byName, def = r.byPath, r.byName, r.def
		r.mu.RUnlock()
	}
	return dropTunnelled(func(pid uint32) bool {
		role := def
		if r != nil {
			if ident, ok := r.identFor(pid); ok {
				role = r.roleOf(ident, byPath, byName, def)
			}
		}
		return role == clientstate.RoleTunnel
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
