//go:build linux

package main

import (
	"encoding/binary"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type routeWatcher struct {
	fd      int
	changed chan struct{}
	done    chan struct{}
	once    sync.Once
}

func watchRoutes() (*routeWatcher, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return nil, err
	}
	groups := uint32(unix.RTMGRP_LINK | unix.RTMGRP_IPV4_IFADDR | unix.RTMGRP_IPV6_IFADDR |
		unix.RTMGRP_IPV4_ROUTE | unix.RTMGRP_IPV6_ROUTE)
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK, Groups: groups}); err != nil {
		unix.Close(fd)
		return nil, err
	}
	tv := unix.NsecToTimeval(int64(time.Second))
	unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv)

	w := &routeWatcher{fd: fd, changed: make(chan struct{}, 1), done: make(chan struct{})}
	go w.read()
	return w, nil
}

func (w *routeWatcher) read() {
	defer unix.Close(w.fd)
	buf := make([]byte, 1<<16)
	for {
		select {
		case <-w.done:
			return
		default:
		}
		n, _, err := unix.Recvfrom(w.fd, buf, 0)
		if err != nil {
			if err == unix.EAGAIN || err == unix.EINTR {
				continue
			}
			return
		}
		msgs, err := syscall.ParseNetlinkMessage(buf[:n])
		if err != nil {
			continue
		}
		for _, m := range msgs {
			if foreign(m) {
				select {
				case w.changed <- struct{}{}:
				default:
				}
				break
			}
		}
	}
}

func foreign(m syscall.NetlinkMessage) bool {
	own := int(tunIndex.Load())
	switch m.Header.Type {
	case unix.RTM_NEWLINK, unix.RTM_DELLINK:
		if len(m.Data) < unix.SizeofIfInfomsg {
			return false
		}
		return own == 0 || int(int32(binary.NativeEndian.Uint32(m.Data[4:8]))) != own
	case unix.RTM_NEWADDR, unix.RTM_DELADDR:
		if len(m.Data) < unix.SizeofIfAddrmsg {
			return false
		}
		return own == 0 || int(binary.NativeEndian.Uint32(m.Data[4:8])) != own
	case unix.RTM_NEWROUTE, unix.RTM_DELROUTE:
		if len(m.Data) < unix.SizeofRtMsg {
			return false
		}
		table := uint32(m.Data[4])
		oif := 0
		attrs, _ := syscall.ParseNetlinkRouteAttr(&m)
		for _, a := range attrs {
			switch a.Attr.Type {
			case unix.RTA_TABLE:
				if len(a.Value) >= 4 {
					table = binary.NativeEndian.Uint32(a.Value)
				}
			case unix.RTA_OIF:
				if len(a.Value) >= 4 {
					oif = int(binary.NativeEndian.Uint32(a.Value))
				}
			}
		}
		if table == routeTable || table == unix.RT_TABLE_LOCAL {
			return false
		}
		return own == 0 || oif != own
	}
	return false
}

func (w *routeWatcher) Changed() <-chan struct{} { return w.changed }

func (w *routeWatcher) Close() error {
	w.once.Do(func() { close(w.done) })
	return nil
}
