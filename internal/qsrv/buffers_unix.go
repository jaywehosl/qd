//go:build unix

package qsrv

import (
	"net"
	"syscall"
)

// checkBuffers сверяет, что ядро дало буфер, который узел просил. Потолок
// net.core.rmem_max по умолчанию 208 КБ — на 800 Мбит это меньше двух
// миллисекунд, и всплеск теряется на сокете, не дойдя до QUIC. Молча это не
// видно ничем, кроме просевшей скорости.
func (n *Node) checkBuffers(udp *net.UDPConn, want int) {
	raw, err := udp.SyscallConn()
	if err != nil {
		return
	}
	var got int
	raw.Control(func(fd uintptr) {
		got, _ = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
	})
	// Ядро отдаёт вдвое больше запрошенного: половина — накладные расходы.
	if got > 0 && got/2 < want {
		n.cfg.Log("socket    kernel capped the buffer at %d KiB of %d KiB asked; raise net.core.rmem_max and net.core.wmem_max",
			got/2/1024, want/1024)
	}
}
