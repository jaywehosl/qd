//go:build windows

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/qcli/windivert"
	"golang.org/x/sys/windows"
)

var (
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable = iphlpapi.NewProc("GetExtendedUdpTable")
)

const (
	tcpTableOwnerPidAll = 5
	udpTableOwnerPid    = 1
)

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

func readSockets(into map[portKey]uint32) {
	readTCPTable(into)
	readUDPTable(into)
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

func splitRules(r *procRouter) {}
