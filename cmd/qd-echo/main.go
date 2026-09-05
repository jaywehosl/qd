package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type counters struct {
	sent     atomic.Uint64
	recv     atomic.Uint64
	sentByte atomic.Uint64
	recvByte atomic.Uint64
	sendErr  atomic.Uint64
	rttSum   atomic.Uint64
	rttCount atomic.Uint64
}

func main() {
	server := flag.String("server", "", "echo server host:port")
	size := flag.Int("size", 1400, "udp payload size")
	pps := flag.Int("pps", 0, "target packets per second in total, 0 = as fast as possible")
	senders := flag.Int("senders", 1, "parallel sockets")
	duration := flag.Duration("duration", 10*time.Second, "test length")
	sockbuf := flag.Int("sockbuf", 8<<20, "socket buffer size")
	quiet := flag.Bool("quiet", false, "only print the summary")
	flag.Parse()

	if *server == "" {
		fmt.Fprintln(os.Stderr, "usage: qd-echo -server host:51820 [-size 1400] [-pps 0] [-senders 1] [-duration 10s]")
		os.Exit(2)
	}
	if *size < 16 {
		*size = 16
	}
	if *senders < 1 {
		*senders = 1
	}

	addr, err := net.ResolveUDPAddr("udp4", *server)
	if err != nil {
		fatal("resolve: %v", err)
	}

	conns := make([]*net.UDPConn, 0, *senders)
	for i := 0; i < *senders; i++ {
		c, err := net.DialUDP("udp4", nil, addr)
		if err != nil {
			fatal("dial %d: %v", i, err)
		}
		c.SetReadBuffer(*sockbuf)
		c.SetWriteBuffer(*sockbuf)
		defer c.Close()
		conns = append(conns, c)
	}

	var c counters
	stop := make(chan struct{})

	perSender := 0
	if *pps > 0 {
		perSender = *pps / *senders
	}

	fmt.Printf("target   %s\n", addr)
	fmt.Printf("payload  %d bytes (%d on the wire)\n", *size, *size+42)
	fmt.Printf("sockets  %d\n", *senders)
	if *pps > 0 {
		fmt.Printf("rate     %d pps total = %.0f Mbit/s on the wire\n", *pps, float64(*pps)*float64(*size+42)*8/1e6)
	} else {
		fmt.Printf("rate     unlimited\n")
	}
	fmt.Println()

	for _, conn := range conns {
		go receiver(conn, &c, stop)
	}
	if !*quiet {
		go report(&c, stop)
	}

	var wg sync.WaitGroup
	for _, conn := range conns {
		wg.Add(1)
		go func(conn *net.UDPConn) {
			defer wg.Done()
			sender(conn, &c, *size, perSender, *duration)
		}(conn)
	}
	wg.Wait()

	time.Sleep(300 * time.Millisecond)
	close(stop)

	summary(&c, *duration)
}

func sender(conn *net.UDPConn, c *counters, size, pps int, duration time.Duration) {
	buf := make([]byte, size)
	for i := 16; i < size; i++ {
		buf[i] = byte(i)
	}

	deadline := time.Now().Add(duration)
	var seq uint64

	var interval time.Duration
	if pps > 0 {
		interval = time.Second / time.Duration(pps)
	}
	next := time.Now()

	for time.Now().Before(deadline) {
		seq++
		binary.BigEndian.PutUint64(buf[0:8], seq)
		binary.BigEndian.PutUint64(buf[8:16], uint64(time.Now().UnixNano()))

		n, err := conn.Write(buf)
		if err != nil {
			c.sendErr.Add(1)
			continue
		}
		c.sent.Add(1)
		c.sentByte.Add(uint64(n))

		if interval > 0 {
			next = next.Add(interval)
			if wait := time.Until(next); wait > 0 {
				time.Sleep(wait)
			}
		}
	}
}

func receiver(conn *net.UDPConn, c *counters, stop <-chan struct{}) {
	buf := make([]byte, 65535)
	for {
		select {
		case <-stop:
			return
		default:
		}
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, err := conn.Read(buf)
		if err != nil {
			continue
		}
		c.recv.Add(1)
		c.recvByte.Add(uint64(n))

		if n >= 16 {
			sent := int64(binary.BigEndian.Uint64(buf[8:16]))
			if rtt := time.Now().UnixNano() - sent; rtt > 0 && rtt < int64(time.Second) {
				c.rttSum.Add(uint64(rtt))
				c.rttCount.Add(1)
			}
		}
	}
}

func report(c *counters, stop <-chan struct{}) {
	t := time.NewTicker(time.Second)
	defer t.Stop()

	var lastSent, lastRecv, lastSentB, lastRecvB uint64
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s, r := c.sent.Load(), c.recv.Load()
			sb, rb := c.sentByte.Load(), c.recvByte.Load()

			ds, dr := s-lastSent, r-lastRecv
			dsb, drb := sb-lastSentB, rb-lastRecvB
			lastSent, lastRecv, lastSentB, lastRecvB = s, r, sb, rb

			loss := 0.0
			if ds > 0 {
				loss = 100 * (1 - float64(dr)/float64(ds))
				if loss < 0 {
					loss = 0
				}
			}
			fmt.Printf("tx %6d pps %7.1f Mbit/s | rx %6d pps %7.1f Mbit/s | loss %5.1f%% | rtt %s\n",
				ds, wire(dsb, ds), dr, wire(drb, dr), loss, avgRTT(c))
		}
	}
}

func wire(bytes, packets uint64) float64 {
	return float64(bytes+42*packets) * 8 / 1e6
}

func avgRTT(c *counters) string {
	n := c.rttCount.Load()
	if n == 0 {
		return "n/a"
	}
	return (time.Duration(c.rttSum.Load()/n) * time.Nanosecond).Round(10 * time.Microsecond).String()
}

func summary(c *counters, d time.Duration) {
	s, r := c.sent.Load(), c.recv.Load()
	secs := d.Seconds()

	loss := 0.0
	if s > 0 {
		loss = 100 * (1 - float64(r)/float64(s))
		if loss < 0 {
			loss = 0
		}
	}

	fmt.Println()
	fmt.Printf("sent     %d packets, %.0f pps, %.1f Mbit/s\n",
		s, float64(s)/secs, wire(c.sentByte.Load(), s)/secs)
	fmt.Printf("received %d packets, %.0f pps, %.1f Mbit/s\n",
		r, float64(r)/secs, wire(c.recvByte.Load(), r)/secs)
	fmt.Printf("loss     %.2f%%\n", loss)
	fmt.Printf("rtt      %s average\n", avgRTT(c))
	if e := c.sendErr.Load(); e > 0 {
		fmt.Printf("send errors %d\n", e)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
