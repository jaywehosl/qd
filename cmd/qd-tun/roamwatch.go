//go:build windows

package main

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	procNotifyRouteChange2      = iphlpapi.NewProc("NotifyRouteChange2")
	procNotifyIpInterfaceChange = iphlpapi.NewProc("NotifyIpInterfaceChange")
	procCancelMibChangeNotify2  = iphlpapi.NewProc("CancelMibChangeNotify2")
)

type routeWatcher struct {
	changed chan struct{}

	mu       sync.Mutex
	handles  []syscall.Handle
	callback uintptr
	closed   bool
}

func watchRoutes() (*routeWatcher, error) {
	w := &routeWatcher{changed: make(chan struct{}, 1)}
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

func (w *routeWatcher) onChange(_ uintptr, _ uintptr, _ uintptr) uintptr {
	select {
	case w.changed <- struct{}{}:
	default:
	}
	return 0
}

func (w *routeWatcher) Changed() <-chan struct{} { return w.changed }

func (w *routeWatcher) Close() error {
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
