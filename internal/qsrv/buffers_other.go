//go:build !unix

package qsrv

import "net"

func (n *Node) checkBuffers(*net.UDPConn, int) {}
