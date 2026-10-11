//go:build unix

package qsrv

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

const tcpMemPath = "/proc/sys/net/ipv4/tcp_mem"

func (n *Node) checkBuffers(udp *net.UDPConn, want int) {
	raw, err := udp.SyscallConn()
	if err != nil {
		return
	}
	var got int
	raw.Control(func(fd uintptr) {
		got, _ = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
	})
	if got > 0 && got/2 < want {
		n.cfg.Log("socket    kernel capped the buffer at %d KiB of %d KiB asked; raise net.core.rmem_max and net.core.wmem_max",
			got/2/1024, want/1024)
	}
}

func (n *Node) roomForTCP() {
	raw, err := os.ReadFile(tcpMemPath)
	if err != nil {
		return
	}
	var low, pressure, high uint64
	if _, err := fmt.Sscan(string(raw), &low, &pressure, &high); err != nil {
		return
	}
	pages, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return
	}
	var total uint64
	if _, err := fmt.Sscanf(string(pages), "MemTotal: %d kB", &total); err != nil {
		return
	}
	total = total * 1024 / uint64(os.Getpagesize())
	want := [3]uint64{total / 8, total / 4, total * 3 / 8}
	if pressure >= want[1] {
		return
	}
	if err := os.WriteFile(tcpMemPath, []byte(fmt.Sprintf("%d %d %d", want[0], want[1], want[2])), 0o644); err != nil {
		n.cfg.Log("socket    could not raise the kernel tcp memory ceiling: %v", err)
		return
	}
	page := uint64(os.Getpagesize())
	n.cfg.Log("socket    kernel tcp memory ceiling raised from %d MiB to %d MiB", pressure*page>>20, want[1]*page>>20)
}
