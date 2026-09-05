//go:build unix

package qsrv

import (
	"net"
	"syscall"
)

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
