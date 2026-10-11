//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/qcli/guard"
	"github.com/jaywehosl/qd/internal/qcli/windivert"
	"golang.org/x/sys/windows"
)

var (
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable = iphlpapi.NewProc("GetExtendedUdpTable")

	self = uint32(os.Getpid())
)

const (
	tcpTableOwnerPidAll = 5
	udpTableOwnerPid    = 1

	errInsufficientBuffer = 122
	systemPid             = 4
	processIdInformation  = 88

	flushWait = 50 * time.Millisecond
)

type platformRouter struct {
	watching atomic.Int32
	flushing atomic.Int32

	readMu sync.Mutex
	buf    []byte

	echoMu   sync.Mutex
	echoCond *sync.Cond
	echoed   map[uint16]bool
}

func (r *procRouter) start() {
	r.plat.echoCond = sync.NewCond(&r.plat.echoMu)
	r.plat.echoed = map[uint16]bool{}
}

func (r *procRouter) watchSockets(ctx context.Context, dll string) {
	watch, err := windivert.WatchSockets(dll)
	if err != nil {
		fmt.Printf("routing  no socket watch, closed sockets are told by the windows tables alone: %v\n", err)
		return
	}
	defer watch.Close()

	r.owners.Clear()
	r.plat.watching.Add(1)
	defer r.plat.watching.Add(-1)
	fmt.Printf("routing  socket watch is up\n")

	err = watch.Watch(ctx, r.heard)
	if ctx.Err() == nil {
		fmt.Printf("routing  socket watch stopped: %v\n", err)
	}
}

func (r *procRouter) heard(event uint8, d windivert.SocketData) {
	if d.ProcessID == 0 || (d.Protocol != protoTCP && d.Protocol != protoUDP) {
		return
	}
	if d.LocalAddr.IsLoopback() {
		if d.ProcessID == self && event == windivert.EventSocketBind && r.plat.flushing.Load() > 0 {
			r.plat.echoMu.Lock()
			r.plat.echoed[d.LocalPort] = true
			r.plat.echoCond.Broadcast()
			r.plat.echoMu.Unlock()
		}
		return
	}

	key := portKey{proto: d.Protocol, v6: d.LocalAddr.Is6(), port: d.LocalPort}
	var o owner
	held, seen := r.owners.Load(key)
	if seen {
		o = held.(owner)
	}

	if event == windivert.EventSocketClose {
		if d.RemotePort != 0 || !seen || (o.pid != d.ProcessID && o.by != d.ProcessID) {
			return
		}
		if o.pid == 0 {
			o.pid, o.bound = o.by, o.ident
		}
		o.closed, o.closedAt, o.decided = true, time.Now().UnixMilli(), false
	} else {
		if seen && o.pid == d.ProcessID && !o.closed {
			return
		}
		o = owner{pid: d.ProcessID, bound: identOf(d.ProcessID)}
	}
	r.owners.Store(key, o)
}

func (r *procRouter) flush() {
	p := &r.plat
	if p.watching.Load() == 0 {
		return
	}
	p.flushing.Add(1)
	defer p.flushing.Add(-1)

	probe, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return
	}
	defer probe.Close()
	port := uint16(probe.LocalAddr().(*net.UDPAddr).Port)

	deadline := time.Now().Add(flushWait)
	wake := time.AfterFunc(flushWait, func() {
		p.echoMu.Lock()
		p.echoCond.Broadcast()
		p.echoMu.Unlock()
	})
	defer wake.Stop()

	p.echoMu.Lock()
	for !p.echoed[port] && time.Now().Before(deadline) {
		p.echoCond.Wait()
	}
	delete(p.echoed, port)
	p.echoMu.Unlock()
}

type tableShape struct {
	proc                     *syscall.LazyProc
	family, class            uintptr
	row, addr, addrLen, port int
	pid                      int
}

func shapeOf(key portKey) tableShape {
	switch {
	case key.proto == protoTCP && !key.v6:
		return tableShape{procGetExtendedTcpTable, afInet, tcpTableOwnerPidAll, 24, 4, 4, 8, 20}
	case key.proto == protoTCP:
		return tableShape{procGetExtendedTcpTable, afInet6, tcpTableOwnerPidAll, 56, 0, 16, 20, 52}
	case !key.v6:
		return tableShape{procGetExtendedUdpTable, afInet, udpTableOwnerPid, 12, 0, 4, 4, 8}
	}
	return tableShape{procGetExtendedUdpTable, afInet6, udpTableOwnerPid, 28, 0, 16, 20, 24}
}

func (r *procRouter) pidOf(key portKey, src netip.Addr) (pid uint32, found, read bool) {
	p := &r.plat
	p.readMu.Lock()
	defer p.readMu.Unlock()

	shape := shapeOf(key)
	table, ok := p.read(shape)
	if !ok {
		return 0, false, false
	}

	var exact []byte
	if src.IsValid() && src.Is6() == key.v6 {
		exact = src.AsSlice()
	}
	none := make([]byte, shape.addrLen)

	best := 0
	rows := int(binary.LittleEndian.Uint32(table[0:4]))
	for i := 0; i < rows; i++ {
		row := table[4+i*shape.row:]
		if len(row) < shape.row {
			break
		}
		if binary.BigEndian.Uint16(row[shape.port:]) != key.port {
			continue
		}
		holder := binary.LittleEndian.Uint32(row[shape.pid:])
		local := row[shape.addr : shape.addr+shape.addrLen]
		rank := 1
		switch {
		case holder == 0, loopbackBytes(local):
			continue
		case bytes.Equal(local, exact):
			rank = 3
		case bytes.Equal(local, none):
			rank = 2
		}
		if rank > best {
			best, pid = rank, holder
		}
	}
	return pid, best > 0, true
}

func loopbackBytes(addr []byte) bool {
	if len(addr) == 4 {
		return addr[0] == 127
	}
	return addr[15] == 1 && bytes.Equal(addr[:15], make([]byte, 15))
}

func (p *platformRouter) read(shape tableShape) ([]byte, bool) {
	if p.buf == nil {
		p.buf = make([]byte, 64<<10)
	}
	for try := 0; try < 3; try++ {
		size := uint32(len(p.buf))
		rc, _, _ := shape.proc.Call(
			uintptr(unsafe.Pointer(&p.buf[0])),
			uintptr(unsafe.Pointer(&size)),
			0, shape.family, shape.class, 0,
		)
		switch rc {
		case 0:
			return p.buf, true
		case errInsufficientBuffer:
			p.buf = make([]byte, int(size)+16<<10)
		default:
			return nil, false
		}
	}
	return nil, false
}

func extendedTable(proc *syscall.LazyProc, class uintptr) ([]byte, bool) {
	var size uint32
	proc.Call(0, uintptr(unsafe.Pointer(&size)), 0, afInet, class, 0)
	if size == 0 {
		return nil, false
	}
	buf := make([]byte, size+4096)
	size = uint32(len(buf))
	r, _, _ := proc.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		0, afInet, class, 0,
	)
	if r != 0 || size < 4 {
		return nil, false
	}
	return buf[:size], true
}

func lookupProcess(pid uint32) procIdent {
	if pid == systemPid {
		return procIdent{name: "System"}
	}
	full := imagePath(pid)
	if full == "" {
		full = dosPath(kernelImagePath(pid))
	}
	if full == "" {
		return procIdent{}
	}
	return procIdent{name: filepath.Base(full), path: full}
}

func imagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

func kernelImagePath(pid uint32) string {
	buf := make([]uint16, 1024)
	info := struct {
		pid   uintptr
		image windows.NTUnicodeString
	}{
		pid:   uintptr(pid),
		image: windows.NTUnicodeString{MaximumLength: uint16(len(buf) * 2), Buffer: &buf[0]},
	}
	if windows.NtQuerySystemInformation(processIdInformation, unsafe.Pointer(&info), uint32(unsafe.Sizeof(info)), nil) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:info.image.Length/2])
}

func dosPath(device string) string {
	if device == "" {
		return ""
	}
	drives, _ := windows.GetLogicalDrives()
	target := make([]uint16, 512)
	for letter := 0; letter < 26; letter++ {
		if drives&(1<<letter) == 0 {
			continue
		}
		drive := string(rune('A'+letter)) + ":"
		name, _ := windows.UTF16PtrFromString(drive)
		if n, err := windows.QueryDosDevice(name, &target[0], uint32(len(target))); err != nil || n == 0 {
			continue
		}
		if rest, ok := strings.CutPrefix(device, windows.UTF16ToString(target)+`\`); ok {
			return drive + `\` + rest
		}
	}
	return device
}

const tcpStateDeleteTCB = 12

var procSetTcpEntry = iphlpapi.NewProc("SetTcpEntry")

type tcpRow struct {
	state      uint32
	localAddr  uint32
	localPort  uint32
	remoteAddr uint32
	remotePort uint32
	pid        uint32
}

func tcpRowsWithPid() []tcpRow {
	buf, ok := extendedTable(procGetExtendedTcpTable, tcpTableOwnerPidAll)
	if !ok {
		return nil
	}
	n := binary.LittleEndian.Uint32(buf[0:4])
	out := make([]tcpRow, 0, n)
	const rowSize = 24
	for i := uint32(0); i < n; i++ {
		off := 4 + int(i)*rowSize
		if off+rowSize > len(buf) {
			break
		}
		out = append(out, tcpRow{
			state:      binary.LittleEndian.Uint32(buf[off : off+4]),
			localAddr:  binary.LittleEndian.Uint32(buf[off+4 : off+8]),
			localPort:  binary.LittleEndian.Uint32(buf[off+8 : off+12]),
			remoteAddr: binary.LittleEndian.Uint32(buf[off+12 : off+16]),
			remotePort: binary.LittleEndian.Uint32(buf[off+16 : off+20]),
			pid:        binary.LittleEndian.Uint32(buf[off+20 : off+24]),
		})
	}
	return out
}

func dropConnection(r tcpRow) bool {
	row := struct {
		state      uint32
		localAddr  uint32
		localPort  uint32
		remoteAddr uint32
		remotePort uint32
	}{
		state:      tcpStateDeleteTCB,
		localAddr:  r.localAddr,
		localPort:  r.localPort,
		remoteAddr: r.remoteAddr,
		remotePort: r.remotePort,
	}
	rc, _, _ := procSetTcpEntry.Call(uintptr(unsafe.Pointer(&row)))
	return rc == 0
}

func openRemotes() []netip.Addr {
	rows := tcpRowsWithPid()
	out := make([]netip.Addr, 0, len(rows))
	for _, row := range rows {
		if addr := row.remoteAddr; addr != 0 {
			out = append(out, netip.AddrFrom4([4]byte{byte(addr), byte(addr >> 8), byte(addr >> 16), byte(addr >> 24)}))
		}
	}
	return out
}

func dropMoved(moved []netip.Addr) int {
	if len(moved) == 0 {
		return 0
	}
	hit := make(map[netip.Addr]bool, len(moved))
	for _, a := range moved {
		hit[a] = true
	}
	dropped := 0
	for _, row := range tcpRowsWithPid() {
		addr := row.remoteAddr
		if row.pid == self || addr == 0 {
			continue
		}
		if hit[netip.AddrFrom4([4]byte{byte(addr), byte(addr >> 8), byte(addr >> 16), byte(addr >> 24)})] && dropConnection(row) {
			dropped++
		}
	}
	return dropped
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
	dropped := 0
	for _, row := range tcpRowsWithPid() {
		if row.pid == 0 || row.pid == self || row.remoteAddr == 0 || goesAround(row.remoteAddr) {
			continue
		}
		ident, ok := identFor(row.pid)
		if !ok {
			continue
		}
		if roleIn(ident, oldPath, oldName, oldDef) == roleIn(ident, newPath, newName, newDef) {
			continue
		}
		if dropConnection(row) {
			dropped++
		}
	}
	return dropped
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
	dropped := 0
	for _, row := range tcpRowsWithPid() {
		if row.pid == 0 || row.pid == self || row.remoteAddr == 0 || goesAround(row.remoteAddr) {
			continue
		}
		role := def
		if ident, ok := identFor(row.pid); ok {
			role = roleIn(ident, byPath, byName, def)
		}
		if !pick(role) {
			continue
		}
		if dropConnection(row) {
			dropped++
		}
	}
	return dropped
}

func goesAround(addr uint32) bool {
	to := netip.AddrFrom4([4]byte{byte(addr), byte(addr >> 8), byte(addr >> 16), byte(addr >> 24)})
	if around.Bypass(to) {
		return true
	}
	for _, list := range []*[]netip.Prefix{keptOut.Load(), lateAside.Load()} {
		if list == nil {
			continue
		}
		for _, p := range *list {
			if p.Contains(to) {
				return true
			}
		}
	}
	return false
}

var (
	around  = guard.New(nil)
	keptOut atomic.Pointer[[]netip.Prefix]
)

func splitRules(r *procRouter) {}
