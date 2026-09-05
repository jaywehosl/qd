//go:build windows

package wintun

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	dll = syscall.NewLazyDLL(unpack())

	procCreateAdapter        = dll.NewProc("WintunCreateAdapter")
	procOpenAdapter          = dll.NewProc("WintunOpenAdapter")
	procCloseAdapter         = dll.NewProc("WintunCloseAdapter")
	procGetAdapterLUID       = dll.NewProc("WintunGetAdapterLUID")
	procStartSession         = dll.NewProc("WintunStartSession")
	procEndSession           = dll.NewProc("WintunEndSession")
	procGetReadWaitEvent     = dll.NewProc("WintunGetReadWaitEvent")
	procReceivePacket        = dll.NewProc("WintunReceivePacket")
	procReleaseReceivePacket = dll.NewProc("WintunReleaseReceivePacket")
	procAllocateSendPacket   = dll.NewProc("WintunAllocateSendPacket")
	procSendPacket           = dll.NewProc("WintunSendPacket")

	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	procWaitForSingle   = kernel32.NewProc("WaitForSingleObject")
	procWaitForMultiple = kernel32.NewProc("WaitForMultipleObjects")
	procCreateEvent     = kernel32.NewProc("CreateEventW")
	procSetEvent        = kernel32.NewProc("SetEvent")
	procResetEvent      = kernel32.NewProc("ResetEvent")
	procCloseHandle     = kernel32.NewProc("CloseHandle")
)

const (
	MinRingCapacity = 0x20000
	MaxRingCapacity = 0x4000000

	errNoMoreItems = syscall.Errno(259)
	waitTimeout    = 0x102
	waitObject0    = 0x0
	infinite       = 0xFFFFFFFF
)

type Adapter struct {
	handle uintptr
}

type Session struct {
	handle    uintptr
	readEvent uintptr
	stopEvent uintptr
}

func CreateAdapter(name, tunnelType string, guid *[16]byte) (*Adapter, error) {
	name16, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	type16, err := syscall.UTF16PtrFromString(tunnelType)
	if err != nil {
		return nil, err
	}

	var guidPtr uintptr
	if guid != nil {
		guidPtr = uintptr(unsafe.Pointer(guid))
	}

	h, _, errno := procCreateAdapter.Call(
		uintptr(unsafe.Pointer(name16)),
		uintptr(unsafe.Pointer(type16)),
		guidPtr,
	)
	if h == 0 {
		return nil, fmt.Errorf("wintun: create adapter %q: %w", name, errno)
	}
	return &Adapter{handle: h}, nil
}

func OpenAdapter(name string) (*Adapter, error) {
	name16, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	h, _, errno := procOpenAdapter.Call(uintptr(unsafe.Pointer(name16)))
	if h == 0 {
		return nil, fmt.Errorf("wintun: open adapter %q: %w", name, errno)
	}
	return &Adapter{handle: h}, nil
}

func (a *Adapter) Close() {
	if a.handle != 0 {
		procCloseAdapter.Call(a.handle)
		a.handle = 0
	}
}

func (a *Adapter) LUID() uint64 {
	var luid uint64
	procGetAdapterLUID.Call(a.handle, uintptr(unsafe.Pointer(&luid)))
	return luid
}

func (a *Adapter) StartSession(capacity uint32) (*Session, error) {
	if capacity < MinRingCapacity {
		capacity = MinRingCapacity
	}
	if capacity > MaxRingCapacity {
		capacity = MaxRingCapacity
	}

	h, _, errno := procStartSession.Call(a.handle, uintptr(capacity))
	if h == 0 {
		return nil, fmt.Errorf("wintun: start session: %w", errno)
	}

	ev, _, _ := procGetReadWaitEvent.Call(h)
	stop, _, _ := procCreateEvent.Call(0, 1, 0, 0)
	return &Session{handle: h, readEvent: ev, stopEvent: stop}, nil
}

func (s *Session) Interrupt() {
	if s.stopEvent != 0 {
		procSetEvent.Call(s.stopEvent)
	}
}

func (s *Session) Resume() {
	if s.stopEvent != 0 {
		procResetEvent.Call(s.stopEvent)
	}
}

func (s *Session) End() {
	s.Interrupt()
	if s.handle != 0 {
		procEndSession.Call(s.handle)
		s.handle = 0
	}
	if s.stopEvent != 0 {
		procCloseHandle.Call(s.stopEvent)
		s.stopEvent = 0
	}
}

var ErrClosed = errors.New("wintun: session closed")

func (s *Session) Receive() ([]byte, error) {
	for {
		var size uint32
		ptr, _, errno := procReceivePacket.Call(s.handle, uintptr(unsafe.Pointer(&size)))
		if ptr != 0 {
			return unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size), nil
		}
		if errno != errNoMoreItems {
			return nil, fmt.Errorf("wintun: receive: %w", errno)
		}

		if s.stopEvent == 0 {
			r, _, _ := procWaitForSingle.Call(s.readEvent, infinite)
			if r != 0 {
				return nil, ErrClosed
			}
			continue
		}

		handles := [2]uintptr{s.readEvent, s.stopEvent}
		r, _, _ := procWaitForMultiple.Call(2, uintptr(unsafe.Pointer(&handles[0])), 0, infinite)
		switch r {
		case waitObject0:
			continue
		case waitObject0 + 1:
			return nil, ErrClosed
		default:
			return nil, ErrClosed
		}
	}
}

func (s *Session) Release(packet []byte) {
	if len(packet) == 0 {
		return
	}
	procReleaseReceivePacket.Call(s.handle, uintptr(unsafe.Pointer(&packet[0])))
}

func (s *Session) Send(data []byte) error {
	ptr, _, errno := procAllocateSendPacket.Call(s.handle, uintptr(len(data)))
	if ptr == 0 {
		return fmt.Errorf("wintun: allocate: %w", errno)
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), len(data))
	copy(dst, data)
	procSendPacket.Call(s.handle, ptr)
	return nil
}
