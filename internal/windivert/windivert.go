//go:build windows

package windivert

import (
	"fmt"
	"syscall"
	"unsafe"
)

type Layer uint32

const (
	LayerNetwork Layer = 0
	LayerFlow    Layer = 2
	LayerSocket  Layer = 3
)

type Flag uint64

const (
	FlagSniff     Flag = 0x0001
	FlagDrop      Flag = 0x0002
	FlagRecvOnly  Flag = 0x0004
	FlagSendOnly  Flag = 0x0008
	FlagNoInstall Flag = 0x0010
	FlagFragments Flag = 0x0020
)

type Param uint32

const (
	ParamQueueLength Param = 0
	ParamQueueTime   Param = 1
	ParamQueueSize   Param = 2
)

const (
	QueueLengthMax  = 16384
	QueueTimeMax    = 16000
	QueueSizeMax    = 33554432
	QueueTimeNormal = 2000
)

const (
	ShutdownRecv = 1
	ShutdownSend = 2
	ShutdownBoth = 3
)

const MaxPacketSize = 0xFFFF

var (
	dll = syscall.NewLazyDLL("WinDivert.dll")

	procOpen          = dll.NewProc("WinDivertOpen")
	procRecvEx        = dll.NewProc("WinDivertRecvEx")
	procSend          = dll.NewProc("WinDivertSend")
	procSendEx        = dll.NewProc("WinDivertSendEx")
	procShutdown      = dll.NewProc("WinDivertShutdown")
	procClose         = dll.NewProc("WinDivertClose")
	procSetParam      = dll.NewProc("WinDivertSetParam")
	procCalcChecksums = dll.NewProc("WinDivertHelperCalcChecksums")
	procCompileFilter = dll.NewProc("WinDivertHelperCompileFilter")
)

type Handle struct {
	h syscall.Handle
}

func Open(filter string, layer Layer, priority int16, flags Flag) (*Handle, error) {
	f, err := syscall.BytePtrFromString(filter)
	if err != nil {
		return nil, err
	}

	r, _, errno := procOpen.Call(
		uintptr(unsafe.Pointer(f)),
		uintptr(layer),
		uintptr(priority),
		uintptr(flags),
	)
	if syscall.Handle(r) == syscall.InvalidHandle {
		return nil, fmt.Errorf("windivert: open: %w", errno)
	}
	return &Handle{h: syscall.Handle(r)}, nil
}

func (h *Handle) SetParam(p Param, value uint64) error {
	r, _, errno := procSetParam.Call(uintptr(h.h), uintptr(p), uintptr(value))
	if r == 0 {
		return fmt.Errorf("windivert: set param %d: %w", p, errno)
	}
	return nil
}

func (h *Handle) RecvEx(packet []byte, addrs []Address) (int, int, error) {
	var (
		recvLen uint32
		addrLen = uint32(len(addrs)) * uint32(addressSize)
	)

	r, _, errno := procRecvEx.Call(
		uintptr(h.h),
		uintptr(unsafe.Pointer(&packet[0])),
		uintptr(len(packet)),
		uintptr(unsafe.Pointer(&recvLen)),
		0,
		uintptr(unsafe.Pointer(&addrs[0])),
		uintptr(unsafe.Pointer(&addrLen)),
		0,
	)
	if r == 0 {
		return 0, 0, fmt.Errorf("windivert: recvex: %w", errno)
	}
	return int(recvLen), int(addrLen) / addressSize, nil
}

func (h *Handle) Send(packet []byte, addr *Address) (int, error) {
	var sent uint32
	r, _, errno := procSend.Call(
		uintptr(h.h),
		uintptr(unsafe.Pointer(&packet[0])),
		uintptr(len(packet)),
		uintptr(unsafe.Pointer(&sent)),
		uintptr(unsafe.Pointer(addr)),
	)
	if r == 0 {
		return 0, fmt.Errorf("windivert: send: %w", errno)
	}
	return int(sent), nil
}

func (h *Handle) SendEx(packet []byte, addrs []Address) (int, error) {
	var sent uint32
	r, _, errno := procSendEx.Call(
		uintptr(h.h),
		uintptr(unsafe.Pointer(&packet[0])),
		uintptr(len(packet)),
		uintptr(unsafe.Pointer(&sent)),
		0,
		uintptr(unsafe.Pointer(&addrs[0])),
		uintptr(uint32(len(addrs))*uint32(addressSize)),
		0,
	)
	if r == 0 {
		return 0, fmt.Errorf("windivert: sendex: %w", errno)
	}
	return int(sent), nil
}

func (h *Handle) Shutdown(how uint32) error {
	r, _, errno := procShutdown.Call(uintptr(h.h), uintptr(how))
	if r == 0 {
		return fmt.Errorf("windivert: shutdown: %w", errno)
	}
	return nil
}

func (h *Handle) Close() error {
	r, _, errno := procClose.Call(uintptr(h.h))
	if r == 0 {
		return fmt.Errorf("windivert: close: %w", errno)
	}
	return nil
}

func CalcChecksums(packet []byte, addr *Address, flags uint64) error {
	r, _, errno := procCalcChecksums.Call(
		uintptr(unsafe.Pointer(&packet[0])),
		uintptr(len(packet)),
		uintptr(unsafe.Pointer(addr)),
		uintptr(flags),
	)
	if r == 0 {
		return fmt.Errorf("windivert: checksums: %w", errno)
	}
	return nil
}

func CompileFilter(filter string, layer Layer) error {
	f, err := syscall.BytePtrFromString(filter)
	if err != nil {
		return err
	}
	var (
		errStr *byte
		errPos uint32
	)
	r, _, _ := procCompileFilter.Call(
		uintptr(unsafe.Pointer(f)),
		uintptr(layer),
		0, 0,
		uintptr(unsafe.Pointer(&errStr)),
		uintptr(unsafe.Pointer(&errPos)),
	)
	if r == 0 {
		msg := "invalid filter"
		if errStr != nil {
			msg = bytePtrToString(errStr)
		}
		return fmt.Errorf("windivert: filter at position %d: %s", errPos, msg)
	}
	return nil
}

func bytePtrToString(p *byte) string {
	if p == nil {
		return ""
	}
	var out []byte
	for i := 0; ; i++ {
		c := *(*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + uintptr(i)))
		if c == 0 {
			break
		}
		out = append(out, c)
	}
	return string(out)
}
