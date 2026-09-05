//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"golang.org/x/sys/windows"
)

var (
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable = iphlpapi.NewProc("GetExtendedUdpTable")
)

const (
	tcpTableOwnerPidAll = 5
	udpTableOwnerPid    = 1

	protoTCP = 6
	protoUDP = 17

	tableTTL     = 5 * time.Second
	missCooldown = 40 * time.Millisecond
)

type flowKey struct {
	proto uint8
	port  uint16
}

type procIdent struct {
	name string
	path string
}

type procRouter struct {
	mu     sync.RWMutex
	byPath map[string]string
	byName map[string]string
	def    string

	prevByPath map[string]string
	prevByName map[string]string
	prevDef    string

	active atomic.Bool
	fixed  atomic.Pointer[string]

	decided sync.Map
	unsure  sync.Map

	tbl     atomic.Pointer[map[flowKey]uint32]
	refresh chan struct{}

	pidMu sync.Mutex
	pids  map[uint32]procIdent
}

func newProcRouter() *procRouter {
	return &procRouter{
		byPath:  map[string]string{},
		byName:  map[string]string{},
		def:     clientstate.RoleTunnel,
		refresh: make(chan struct{}, 1),
		pids:    map[uint32]procIdent{},
	}
}

func (r *procRouter) Load(defaultRole string, rules []clientstate.Rule) {
	byPath := map[string]string{}
	byName := map[string]string{}
	for _, rule := range rules {
		if rule.Path != "" {
			byPath[strings.ToLower(rule.Path)] = rule.Role
			continue
		}
		byName[strings.ToLower(rule.Process)] = rule.Role
	}
	if !clientstate.ValidRole(defaultRole) {
		defaultRole = clientstate.RoleTunnel
	}

	r.mu.Lock()
	r.prevByPath, r.prevByName, r.prevDef = r.byPath, r.byName, r.def
	r.byPath, r.byName, r.def = byPath, byName, defaultRole
	r.mu.Unlock()

	r.decided.Range(func(k, _ any) bool {
		r.decided.Delete(k)
		return true
	})

	if len(byPath) == 0 && len(byName) == 0 {
		held := defaultRole
		r.fixed.Store(&held)
		r.active.Store(defaultRole != clientstate.RoleTunnel)
		return
	}
	r.fixed.Store(nil)
	r.active.Store(true)
}

func (r *procRouter) Active() bool { return r.active.Load() }

func (r *procRouter) RoleFor(pkt []byte) string {
	if held := r.fixed.Load(); held != nil {
		return *held
	}

	key, ok := flowOf(pkt)
	if !ok {
		return r.fallback()
	}
	return r.roleForKey(key)
}

func (r *procRouter) RoleForPort(proto uint8, port uint16) string {
	if held := r.fixed.Load(); held != nil {
		return *held
	}
	return r.roleForKey(flowKey{proto: proto, port: port})
}

func (r *procRouter) roleForKey(key flowKey) string {
	if cached, ok := r.decided.Load(key); ok {
		return cached.(string)
	}

	role, sure := r.resolve(key)
	r.decided.Store(key, role)
	if !sure {
		r.unsure.Store(key, struct{}{})
	}
	return role
}

func (r *procRouter) fallback() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.def
}

func (r *procRouter) resolve(key flowKey) (string, bool) {
	pid, ok := r.pidFor(key)
	if !ok {
		st.procMiss.Add(1)
		return r.fallback(), false
	}
	ident, ok := r.identFor(pid)
	if !ok {
		st.procMiss.Add(1)
		return r.fallback(), false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	if ident.path != "" {
		if role, ok := r.byPath[strings.ToLower(ident.path)]; ok {
			return role, true
		}
	}
	if role, ok := r.byName[strings.ToLower(ident.name)]; ok {
		return role, true
	}
	return r.def, true
}

func flowOf(pkt []byte) (flowKey, bool) {
	if len(pkt) < 20 || pkt[0]>>4 != 4 {
		return flowKey{}, false
	}
	proto := pkt[9]
	if proto != protoTCP && proto != protoUDP {
		return flowKey{}, false
	}
	ihl := int(pkt[0]&0x0F) * 4
	if ihl < 20 || len(pkt) < ihl+4 {
		return flowKey{}, false
	}
	return flowKey{proto: proto, port: binary.BigEndian.Uint16(pkt[ihl : ihl+2])}, true
}

func (r *procRouter) pidFor(key flowKey) (uint32, bool) {
	held := r.tbl.Load()
	if held == nil {
		r.wake()
		return 0, false
	}
	pid, ok := (*held)[key]
	if !ok {
		r.wake()
	}
	return pid, ok
}

func (r *procRouter) wake() {
	select {
	case r.refresh <- struct{}{}:
	default:
	}
}

func (r *procRouter) keepTable(stop <-chan struct{}) {
	tick := time.NewTicker(tableTTL)
	defer tick.Stop()

	for {
		r.readTable()

		select {
		case <-stop:
			return
		case <-tick.C:
		case <-r.refresh:
			time.Sleep(missCooldown)
		}
	}
}

func (r *procRouter) readTable() {
	next := make(map[flowKey]uint32, 512)
	readTCPTable(next)
	readUDPTable(next)
	if len(next) == 0 {
		return
	}
	r.tbl.Store(&next)

	r.unsure.Range(func(k, _ any) bool {
		r.unsure.Delete(k)
		r.decided.Delete(k)
		return true
	})
}

func readTCPTable(into map[flowKey]uint32) {
	buf, ok := extendedTable(procGetExtendedTcpTable, tcpTableOwnerPidAll)
	if !ok {
		return
	}
	n := binary.LittleEndian.Uint32(buf[0:4])
	const rowSize = 24
	for i := uint32(0); i < n; i++ {
		off := 4 + int(i)*rowSize
		if off+rowSize > len(buf) {
			return
		}
		port := binary.BigEndian.Uint16(buf[off+8 : off+10])
		pid := binary.LittleEndian.Uint32(buf[off+20 : off+24])
		into[flowKey{proto: protoTCP, port: port}] = pid
	}
}

func readUDPTable(into map[flowKey]uint32) {
	buf, ok := extendedTable(procGetExtendedUdpTable, udpTableOwnerPid)
	if !ok {
		return
	}
	n := binary.LittleEndian.Uint32(buf[0:4])
	const rowSize = 12
	for i := uint32(0); i < n; i++ {
		off := 4 + int(i)*rowSize
		if off+rowSize > len(buf) {
			return
		}
		port := binary.BigEndian.Uint16(buf[off+4 : off+6])
		pid := binary.LittleEndian.Uint32(buf[off+8 : off+12])
		into[flowKey{proto: protoUDP, port: port}] = pid
	}
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

func (r *procRouter) identFor(pid uint32) (procIdent, bool) {
	if pid == 0 {
		return procIdent{}, false
	}

	r.pidMu.Lock()
	ident, ok := r.pids[pid]
	r.pidMu.Unlock()
	if ok {
		return ident, ident.name != ""
	}

	ident = lookupProcess(pid)
	r.pidMu.Lock()
	if len(r.pids) > 4096 {
		r.pids = map[uint32]procIdent{}
	}
	r.pids[pid] = ident
	r.pidMu.Unlock()
	return ident, ident.name != ""
}

func lookupProcess(pid uint32) procIdent {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return procIdent{}
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return procIdent{}
	}
	full := windows.UTF16ToString(buf[:size])
	return procIdent{name: filepath.Base(full), path: full}
}

func (r *procRouter) Forget(pid uint32) {
	r.pidMu.Lock()
	delete(r.pids, pid)
	r.pidMu.Unlock()
}

func reloadProcessRules(db *clientstate.DB) {
	if db == nil {
		return
	}
	r := routeByProcess.Load()
	if r == nil {
		r = newProcRouter()
		routeByProcess.Store(r)
		go r.keepTable(nil)
	}
	rules, err := db.Rules()
	if err != nil {
		return
	}
	def, err := db.DefaultRole()
	if err != nil {
		def = clientstate.RoleTunnel
	}
	r.Load(def, rules)

	if n := r.dropRerouted(); n > 0 {
		fmt.Printf("routing  %d connections dropped so the new rule takes hold now\n", n)
	}
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

func (r *procRouter) roleOf(ident procIdent, byPath, byName map[string]string, def string) string {
	if ident.path != "" {
		if role, ok := byPath[strings.ToLower(ident.path)]; ok {
			return role
		}
	}
	if role, ok := byName[strings.ToLower(ident.name)]; ok {
		return role
	}
	return def
}

func (r *procRouter) dropRerouted() int {
	r.mu.RLock()
	oldPath, oldName, oldDef := r.prevByPath, r.prevByName, r.prevDef
	newPath, newName, newDef := r.byPath, r.byName, r.def
	r.mu.RUnlock()

	if oldPath == nil && oldName == nil {
		return 0
	}

	dropped := 0
	for _, row := range tcpRowsWithPid() {
		if row.pid == 0 || row.remoteAddr == 0 {
			continue
		}
		ident, ok := r.identFor(row.pid)
		if !ok {
			continue
		}
		was := r.roleOf(ident, oldPath, oldName, oldDef)
		now := r.roleOf(ident, newPath, newName, newDef)
		if was == now {
			continue
		}
		if dropConnection(row) {
			dropped++
		}
	}
	return dropped
}

func (r *procRouter) dropInherited() int {
	byPath, byName, def := map[string]string{}, map[string]string{}, clientstate.RoleTunnel
	if r != nil {
		r.mu.RLock()
		byPath, byName, def = r.byPath, r.byName, r.def
		r.mu.RUnlock()
	}

	dropped := 0
	for _, row := range tcpRowsWithPid() {
		if row.pid == 0 || row.remoteAddr == 0 || loopback(row.remoteAddr) {
			continue
		}
		role := def
		if r != nil {
			if ident, ok := r.identFor(row.pid); ok {
				role = r.roleOf(ident, byPath, byName, def)
			}
		}
		if role != clientstate.RoleTunnel {
			continue
		}
		if dropConnection(row) {
			dropped++
		}
	}
	return dropped
}

func loopback(addr uint32) bool { return byte(addr) == 127 }
