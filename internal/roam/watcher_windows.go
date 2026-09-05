//go:build windows

package roam

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	iphlpapi = syscall.NewLazyDLL("iphlpapi.dll")

	procNotifyRouteChange2      = iphlpapi.NewProc("NotifyRouteChange2")
	procNotifyIpInterfaceChange = iphlpapi.NewProc("NotifyIpInterfaceChange")
	procCancelMibChangeNotify2  = iphlpapi.NewProc("CancelMibChangeNotify2")
)

const afUnspec = 0

type systemWatcher struct {
	changed chan struct{}

	mu       sync.Mutex
	handles  []syscall.Handle
	callback uintptr
	closed   bool
}

func NewSystemWatcher() (Watcher, error) {
	w := &systemWatcher{changed: make(chan struct{}, 1)}
	w.callback = syscall.NewCallback(w.onChange)

	var routeHandle, ifaceHandle syscall.Handle
	if r, _, err := procNotifyRouteChange2.Call(
		afUnspec, w.callback, 0, 0, uintptr(unsafe.Pointer(&routeHandle))); r != 0 {
		return nil, err
	}
	if r, _, err := procNotifyIpInterfaceChange.Call(
		afUnspec, w.callback, 0, 0, uintptr(unsafe.Pointer(&ifaceHandle))); r != 0 {
		procCancelMibChangeNotify2.Call(uintptr(routeHandle))
		return nil, err
	}

	w.handles = []syscall.Handle{routeHandle, ifaceHandle}
	return w, nil
}

func (w *systemWatcher) onChange(_ uintptr, _ uintptr, _ uintptr) uintptr {
	select {
	case w.changed <- struct{}{}:
	default:
	}
	return 0
}

func (w *systemWatcher) Changed() <-chan struct{} { return w.changed }

func (w *systemWatcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	for _, h := range w.handles {
		procCancelMibChangeNotify2.Call(uintptr(h))
	}
	w.handles = nil
	return nil
}
