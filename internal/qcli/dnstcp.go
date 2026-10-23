package qcli

import (
	"encoding/binary"
	"io"
	"net"
	"time"
)

func dnsOverTCP(resolver string) (net.Conn, error) {
	udp, err := net.Dial("udp", resolver)
	if err != nil {
		return nil, err
	}
	app, here := net.Pipe()
	go func() {
		defer here.Close()
		defer udp.Close()
		answer := make([]byte, 65535)
		for {
			var size [2]byte
			if _, err := io.ReadFull(here, size[:]); err != nil {
				return
			}
			query := make([]byte, binary.BigEndian.Uint16(size[:]))
			if _, err := io.ReadFull(here, query); err != nil {
				return
			}
			if len(query) < 12 {
				return
			}
			udp.SetDeadline(time.Now().Add(8 * time.Second))
			if _, err := udp.Write(query); err != nil {
				return
			}
			n := 0
			for {
				read, err := udp.Read(answer)
				if err != nil {
					return
				}
				if read >= 2 && answer[0] == query[0] && answer[1] == query[1] {
					n = read
					break
				}
			}
			binary.BigEndian.PutUint16(size[:], uint16(n))
			if _, err := here.Write(append(size[:], answer[:n]...)); err != nil {
				return
			}
		}
	}()
	return app, nil
}
