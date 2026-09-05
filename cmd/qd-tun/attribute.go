//go:build windows

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	windivert "github.com/jaywehosl/quic-diver/internal/qcli/wdsource"
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

	flowCap  = 8192
	flowIdle = 5 * time.Minute
)

type portKey struct {
	proto uint8
	port  uint16
}

type flowKey struct {
	proto uint8
	port  uint16
	dst   netip.Addr
}

type procIdent struct {
	name string
	path string
}

type verdict struct {
	role string
	at   int64
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

	aside      sync.Map
	keptAside  atomic.Int64
	chosen     sync.Map
	keptChosen atomic.Int64

	watched sync.Map
	tbl     atomic.Pointer[map[portKey]uint32]
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

	r.forgetFlows()

	if len(byPath) == 0 && len(byName) == 0 {
		held := defaultRole
		r.fixed.Store(&held)
		r.active.Store(defaultRole != clientstate.RoleTunnel)
		return
	}
	r.fixed.Store(nil)
	r.active.Store(true)
}

func (r *procRouter) forgetFlows() {
	for _, one := range []struct {
		where *sync.Map
		count *atomic.Int64
	}{{&r.aside, &r.keptAside}, {&r.chosen, &r.keptChosen}} {
		one.where.Range(func(k, _ any) bool {
			one.where.Delete(k)
			return true
		})
		one.count.Store(0)
	}
}

func (r *procRouter) Active() bool { return r.active.Load() }

// RoleFor отвечает на вопрос обхода и зовётся из потока захвата, на каждый
// пакет. Ждать здесь нельзя, поэтому решение первого пакета фиксируется как
// есть: пустить уже начатый разговор другой дорогой значит оборвать его чужим
// сбросом.
func (r *procRouter) RoleFor(pkt []byte) string {
	if held := r.fixed.Load(); held != nil {
		return *held
	}

	key, ok := flowOf(pkt)
	if !ok {
		return r.fallback()
	}
	if held, known := r.aside.Load(key); known {
		return held.(verdict).role
	}

	pid, known := r.pidFor(portKey{proto: key.proto, port: key.port})
	role, _ := r.roleOfPid(pid, known)
	r.keep(&r.aside, &r.keptAside, key, role)
	return role
}

// RoleForFlow отвечает на вопрос выхода и зовётся при дозвоне, в своей горутине.
// Там можно подождать хозяина: событие сокета и первый пакет идут разными
// хэндлами драйвера, и порядок между ними не обещан.
//
// Кэш у этого вопроса свой. Общий с обходом не годился: поток захвата спрашивал
// первым и на промахе записывал «не знаю», а дозвон находил готовый ответ и
// хозяина уже не ждал — ожидание не срабатывало ни разу.
func (r *procRouter) RoleForFlow(proto uint8, port uint16, dst netip.Addr) string {
	if held := r.fixed.Load(); held != nil {
		return *held
	}

	key := flowKey{proto: proto, port: port, dst: dst}
	if held, known := r.chosen.Load(key); known {
		return held.(verdict).role
	}

	pid, known := r.awaitOwner(portKey{proto: proto, port: port})
	role, sure := r.roleOfPid(pid, known)
	if sure {
		r.keep(&r.chosen, &r.keptChosen, key, role)
	}
	return role
}

// awaitOwner ждёт хозяина сокета недолго и молча сдаётся: лучше увезти флоу по
// роли по умолчанию, чем держать дозвон.
func (r *procRouter) awaitOwner(key portKey) (uint32, bool) {
	for waited := time.Duration(0); ; waited += ownerStep {
		if pid, ok := r.pidFor(key); ok {
			return pid, true
		}
		if waited >= ownerPatience {
			return 0, false
		}
		time.Sleep(ownerStep)
	}
}

func (r *procRouter) keep(where *sync.Map, count *atomic.Int64, key flowKey, role string) {
	if _, already := where.LoadOrStore(key, verdict{role: role, at: time.Now().UnixMilli()}); already {
		return
	}
	if count.Add(1) > flowCap {
		r.evict(where, count)
	}
}

func (r *procRouter) roleOfPid(pid uint32, known bool) (string, bool) {
	if !known {
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
	return r.roleOf(ident, r.byPath, r.byName, r.def), true
}

const (
	ownerStep     = 2 * time.Millisecond
	ownerPatience = 60 * time.Millisecond
)

func (r *procRouter) evict(where *sync.Map, count *atomic.Int64) {
	cutoff := time.Now().Add(-flowIdle).UnixMilli()
	where.Range(func(key, held any) bool {
		if held.(verdict).at < cutoff {
			where.Delete(key)
			count.Add(-1)
		}
		return true
	})
	if count.Load() <= flowCap {
		return
	}
	where.Range(func(key, _ any) bool {
		where.Delete(key)
		count.Add(-1)
		return count.Load() > flowCap/2
	})
}

func (r *procRouter) fallback() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.def
}

func flowOf(pkt []byte) (flowKey, bool) {
	if len(pkt) < 20 {
		return flowKey{}, false
	}
	var proto byte
	var rest []byte
	var dst netip.Addr
	switch pkt[0] >> 4 {
	case 4:
		ihl := int(pkt[0]&0x0F) * 4
		if ihl < 20 || len(pkt) < ihl+4 {
			return flowKey{}, false
		}
		proto, rest = pkt[9], pkt[ihl:]
		dst = netip.AddrFrom4([4]byte(pkt[16:20]))
	case 6:
		if len(pkt) < 44 {
			return flowKey{}, false
		}
		proto, rest = pkt[6], pkt[40:]
		dst = netip.AddrFrom16([16]byte(pkt[24:40]))
	default:
		return flowKey{}, false
	}
	if proto != protoTCP && proto != protoUDP {
		return flowKey{}, false
	}
	return flowKey{proto: proto, port: binary.BigEndian.Uint16(rest[0:2]), dst: dst}, true
}

func (r *procRouter) pidFor(key portKey) (uint32, bool) {
	if pid, ok := r.watched.Load(key); ok {
		return pid.(uint32), true
	}

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

func (r *procRouter) watchSockets(ctx context.Context, dll string) {
	watch, err := windivert.WatchSockets(dll)
	if err != nil {
		fmt.Printf("routing  no socket watch, falling back to the windows tables: %v\n", err)
		return
	}
	defer watch.Close()
	fmt.Printf("routing  socket watch is up, flow owners are known before the first packet\n")

	err = watch.Watch(ctx, func(event uint8, data windivert.SocketData) {
		if data.Protocol != protoTCP && data.Protocol != protoUDP {
			return
		}
		key := portKey{proto: data.Protocol, port: data.LocalPort}
		if event == windivert.EventSocketClose {
			r.watched.Delete(key)
			return
		}
		if data.ProcessID != 0 {
			r.watched.Store(key, data.ProcessID)
		}
	})
	if ctx.Err() == nil {
		fmt.Printf("routing  socket watch stopped: %v\n", err)
	}
}

// tellMisses называет промахи атрибуции: флоу, чей хозяин так и не нашёлся,
// уезжает по роли по умолчанию — молча это выглядит как «правило не работает».
func (r *procRouter) tellMisses(stop <-chan struct{}) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()

	was := uint64(0)
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
		}
		now := st.procMiss.Load()
		if now > was {
			fmt.Printf("routing  %d flows went by the default role, their owner was not known in time\n", now-was)
			was = now
		}
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
	next := make(map[portKey]uint32, 512)
	readTCPTable(next)
	readUDPTable(next)
	if len(next) == 0 {
		return
	}
	r.tbl.Store(&next)
}

func readTCPTable(into map[portKey]uint32) {
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
		into[portKey{proto: protoTCP, port: port}] = pid
	}
}

func readUDPTable(into map[portKey]uint32) {
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
		into[portKey{proto: protoUDP, port: port}] = pid
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
		go r.tellMisses(nil)
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
	if held := liveTunnel.Load(); held != nil {
		(*held).Reroute()
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
