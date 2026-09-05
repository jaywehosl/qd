//go:build windows

package windivert

import (
	"encoding/binary"
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Layer uint8

const (
	LayerNetwork        Layer = 0
	LayerNetworkForward Layer = 1
	LayerFlow           Layer = 2
	LayerSocket         Layer = 3
	LayerReflect        Layer = 4
)

const (
	FlagSniff     uint64 = 0x0001
	FlagDrop      uint64 = 0x0002
	FlagRecvOnly  uint64 = 0x0004
	FlagSendOnly  uint64 = 0x0008
	FlagNoInstall uint64 = 0x0010
	FlagFragments uint64 = 0x0020
)

const (
	ParamQueueLength uint32 = 0
	ParamQueueTime   uint32 = 1
	ParamQueueSize   uint32 = 2
)

const (
	ShutdownRecv uint32 = 0x1
	ShutdownSend uint32 = 0x2
	ShutdownBoth uint32 = 0x3
)

const (
	BatchMax = 0xFF
	MTUMax   = 40 + 0xFFFF
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

func (a *Address) Layer() Layer   { return Layer(a.word & 0xFF) }
func (a *Address) Event() uint8   { return uint8((a.word >> 8) & 0xFF) }
func (a *Address) Sniffed() bool  { return a.word&(1<<16) != 0 }
func (a *Address) Outbound() bool { return a.word&(1<<17) != 0 }
func (a *Address) Loopback() bool { return a.word&(1<<18) != 0 }
func (a *Address) Impostor() bool { return a.word&(1<<19) != 0 }
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

var (
	loadOnce sync.Once
	loadErr  error

	dll             *windows.DLL
	procOpen        *windows.Proc
	procRecvEx      *windows.Proc
	procSendEx      *windows.Proc
	procClose       *windows.Proc
	procSetParam    *windows.Proc
	procShutdown    *windows.Proc
	procCompileFilt *windows.Proc
	procCalcChecks  *windows.Proc
)

func Load(dllPath string) error {
	loadOnce.Do(func() {
		d, err := windows.LoadDLL(dllPath)
		if err != nil {
			loadErr = fmt.Errorf("load %s: %w", dllPath, err)
			return
		}
		dll = d
		for name, p := range map[string]**windows.Proc{
			"WinDivertOpen":                &procOpen,
			"WinDivertRecvEx":              &procRecvEx,
			"WinDivertSendEx":              &procSendEx,
			"WinDivertClose":               &procClose,
			"WinDivertSetParam":            &procSetParam,
			"WinDivertShutdown":            &procShutdown,
			"WinDivertHelperCompileFilter": &procCompileFilt,
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

func CompileFilter(filter string, layer Layer) (errText string, errPos int, ok bool) {
	fb, err := windows.BytePtrFromString(filter)
	if err != nil {
		return err.Error(), 0, false
	}
	var errStr *byte
	var pos uint32
	r, _, _ := procCompileFilt.Call(
		uintptr(unsafe.Pointer(fb)),
		uintptr(layer),
		0, 0,
		uintptr(unsafe.Pointer(&errStr)),
		uintptr(unsafe.Pointer(&pos)),
	)
	if r != 0 {
		return "", 0, true
	}
	if errStr != nil {
		errText = windows.BytePtrToString(errStr)
	}
	return errText, int(pos), false
}
