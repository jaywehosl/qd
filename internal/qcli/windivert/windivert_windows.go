//go:build windows

package windivert

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Layer uint8

const (
	LayerNetwork Layer = 0
	LayerSocket  Layer = 3
)

const (
	FlagSniff    uint64 = 0x0001
	FlagRecvOnly uint64 = 0x0004
)

const (
	ParamQueueLength uint32 = 0
	ShutdownBoth     uint32 = 0x3
	EventSocketClose uint8  = 8
)

const (
	BatchMax = 0xFF
	addrSize = 80
)

type Address struct {
	Timestamp int64
	word      uint32
	_         uint32
	data      [64]byte
}

func init() {
	if unsafe.Sizeof(Address{}) != addrSize {
		panic("windivert: WINDIVERT_ADDRESS size mismatch")
	}
}

func (a *Address) Event() uint8   { return uint8((a.word >> 8) & 0xFF) }
func (a *Address) Outbound() bool { return a.word&(1<<17) != 0 }
func (a *Address) IPv6() bool     { return a.word&(1<<20) != 0 }

func (a *Address) IPChecksumValid() bool  { return a.word&(1<<21) != 0 }
func (a *Address) TCPChecksumValid() bool { return a.word&(1<<22) != 0 }
func (a *Address) UDPChecksumValid() bool { return a.word&(1<<23) != 0 }

func (a *Address) SetOutbound(v bool) {
	if v {
		a.word |= 1 << 17
	} else {
		a.word &^= 1 << 17
	}
}

func (a *Address) SetLayer(l Layer) { a.word = (a.word &^ 0xFF) | uint32(l) }

func (a *Address) IfIdx() uint32 { return binary.LittleEndian.Uint32(a.data[0:4]) }

func (a *Address) SetIfIdx(idx uint32) { binary.LittleEndian.PutUint32(a.data[0:4], idx) }

type SocketData struct {
	ProcessID  uint32
	LocalAddr  netip.Addr
	RemoteAddr netip.Addr
	LocalPort  uint16
	RemotePort uint16
	Protocol   uint8
}

func (a *Address) Socket() SocketData {
	return SocketData{
		ProcessID:  binary.LittleEndian.Uint32(a.data[16:20]),
		LocalAddr:  addrOf(a.data[20:36], a.IPv6()),
		RemoteAddr: addrOf(a.data[36:52], a.IPv6()),
		LocalPort:  binary.LittleEndian.Uint16(a.data[52:54]),
		RemotePort: binary.LittleEndian.Uint16(a.data[54:56]),
		Protocol:   a.data[56],
	}
}

func addrOf(raw []byte, v6 bool) netip.Addr {
	if !v6 {
		var v4 [4]byte
		binary.BigEndian.PutUint32(v4[:], binary.LittleEndian.Uint32(raw[0:4]))
		return netip.AddrFrom4(v4)
	}
	var v16 [16]byte
	for word := 0; word < 4; word++ {
		binary.BigEndian.PutUint32(v16[12-word*4:16-word*4], binary.LittleEndian.Uint32(raw[word*4:word*4+4]))
	}
	return netip.AddrFrom16(v16)
}

var (
	loadOnce sync.Once
	loadErr  error

	procOpen       *windows.Proc
	procRecvEx     *windows.Proc
	procSendEx     *windows.Proc
	procClose      *windows.Proc
	procSetParam   *windows.Proc
	procShutdown   *windows.Proc
	procCalcChecks *windows.Proc
)

func Load(dllPath string) error {
	loadOnce.Do(func() {
		d, err := windows.LoadDLL(dllPath)
		if err != nil {
			loadErr = fmt.Errorf("load %s: %w", dllPath, err)
			return
		}
		for name, p := range map[string]**windows.Proc{
			"WinDivertOpen":                &procOpen,
			"WinDivertRecvEx":              &procRecvEx,
			"WinDivertSendEx":              &procSendEx,
			"WinDivertClose":               &procClose,
			"WinDivertSetParam":            &procSetParam,
			"WinDivertShutdown":            &procShutdown,
			"WinDivertHelperCalcChecksums": &procCalcChecks,
		} {
			pr, err := d.FindProc(name)
			if err != nil {
				loadErr = fmt.Errorf("proc %s: %w", name, err)
				return
			}
			*p = pr
		}
	})
	return loadErr
}

func open(filter string, layer Layer, priority int16, flags uint64) (windows.Handle, error) {
	fb, err := windows.BytePtrFromString(filter)
	if err != nil {
		return windows.InvalidHandle, err
	}
	r, _, e := procOpen.Call(
		uintptr(unsafe.Pointer(fb)),
		uintptr(layer),
		uintptr(uint16(priority)),
		uintptr(flags),
	)
	h := windows.Handle(r)
	if h == windows.InvalidHandle {
		return h, fmt.Errorf("WinDivertOpen: %w", e)
	}
	return h, nil
}

func recvEx(h windows.Handle, packet []byte, addrs []Address) (packetLen, addrCount uint, err error) {
	var rl uint32
	al := uint32(len(addrs)) * addrSize
	r, _, e := procRecvEx.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&packet[0])),
		uintptr(len(packet)),
		uintptr(unsafe.Pointer(&rl)),
		0,
		uintptr(unsafe.Pointer(&addrs[0])),
		uintptr(unsafe.Pointer(&al)),
		0,
	)
	if r == 0 {
		return 0, 0, fmt.Errorf("WinDivertRecvEx: %w", e)
	}
	return uint(rl), uint(al) / addrSize, nil
}

func sendEx(h windows.Handle, packet []byte, addrs []Address) (sentLen uint, err error) {
	if len(packet) == 0 || len(addrs) == 0 {
		return 0, nil
	}
	var sl uint32
	r, _, e := procSendEx.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&packet[0])),
		uintptr(len(packet)),
		uintptr(unsafe.Pointer(&sl)),
		0,
		uintptr(unsafe.Pointer(&addrs[0])),
		uintptr(uint32(len(addrs))*addrSize),
		0,
	)
	if r == 0 {
		return 0, fmt.Errorf("WinDivertSendEx: %w", e)
	}
	return uint(sl), nil
}

func setParam(h windows.Handle, param uint32, value uint64) error {
	r, _, e := procSetParam.Call(uintptr(h), uintptr(param), uintptr(value))
	if r == 0 {
		return fmt.Errorf("WinDivertSetParam: %w", e)
	}
	return nil
}

func shutdown(h windows.Handle, how uint32) error {
	r, _, e := procShutdown.Call(uintptr(h), uintptr(how))
	if r == 0 {
		return fmt.Errorf("WinDivertShutdown: %w", e)
	}
	return nil
}

func closeHandle(h windows.Handle) error {
	r, _, e := procClose.Call(uintptr(h))
	if r == 0 {
		return fmt.Errorf("WinDivertClose: %w", e)
	}
	return nil
}

func calcChecksums(pkt []byte) {
	if len(pkt) == 0 || procCalcChecks == nil {
		return
	}
	procCalcChecks.Call(
		uintptr(unsafe.Pointer(&pkt[0])),
		uintptr(len(pkt)),
		0,
		0,
	)
}
