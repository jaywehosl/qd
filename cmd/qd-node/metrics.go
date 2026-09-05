//go:build linux

package main

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type sample struct {
	At        int64   `json:"t"`
	CPU       float64 `json:"cpu"`
	Mem       float64 `json:"mem"`
	Swap      float64 `json:"swap"`
	NetUp     float64 `json:"netUp"`
	NetDown   float64 `json:"netDown"`
	PktUp     float64 `json:"pktUp"`
	PktDown   float64 `json:"pktDown"`
	TCPCount  float64 `json:"tcpCount"`
	UDPCount  float64 `json:"udpCount"`
	DiskRead  float64 `json:"diskRead"`
	DiskWrite float64 `json:"diskWrite"`
	DiskUsage float64 `json:"diskUsage"`
	Online    float64 `json:"online"`
	Load1     float64 `json:"load1"`
	Load5     float64 `json:"load5"`
	Load15    float64 `json:"load15"`
}

type counters struct {
	cpuBusy, cpuTotal   uint64
	netUp, netDown      uint64
	pktUp, pktDown      uint64
	diskRead, diskWrite uint64
	at                  time.Time
}

type metrics struct {
	dev                 string
	online              func() int
	memTotal, swapTotal float64

	mu     sync.RWMutex
	ring   []sample
	last   counters
	latest sample
}

const metricsRing = 3600

func startMetrics(dev string, online func() int) *metrics {
	m := &metrics{dev: dev, online: online, ring: make([]sample, 0, metricsRing)}
	m.last = m.read()
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			m.tick()
		}
	}()
	return m
}

func (m *metrics) tick() {
	now := m.read()
	elapsed := now.at.Sub(m.last.at).Seconds()
	if elapsed <= 0 {
		return
	}

	s := sample{At: now.at.Unix()}
	if d := now.cpuTotal - m.last.cpuTotal; d > 0 {
		s.CPU = 100 * float64(now.cpuBusy-m.last.cpuBusy) / float64(d)
	}
	s.NetUp = float64(now.netUp-m.last.netUp) / elapsed
	s.NetDown = float64(now.netDown-m.last.netDown) / elapsed
	s.PktUp = float64(now.pktUp-m.last.pktUp) / elapsed
	s.PktDown = float64(now.pktDown-m.last.pktDown) / elapsed
	s.DiskRead = float64(now.diskRead-m.last.diskRead) / elapsed
	s.DiskWrite = float64(now.diskWrite-m.last.diskWrite) / elapsed

	total, avail := meminfo("MemTotal:", "MemAvailable:")
	if total > 0 {
		s.Mem = 100 * (total - avail) / total
		m.memTotal = total
	}
	swapTotal, swapFree := meminfo("SwapTotal:", "SwapFree:")
	if swapTotal > 0 {
		s.Swap = 100 * (swapTotal - swapFree) / swapTotal
		m.swapTotal = swapTotal
	}

	s.TCPCount = float64(sockets("/proc/net/tcp") + sockets("/proc/net/tcp6"))
	s.UDPCount = float64(sockets("/proc/net/udp") + sockets("/proc/net/udp6"))
	s.DiskUsage = diskUsage("/")
	s.Load1, s.Load5, s.Load15 = loadavg()
	if m.online != nil {
		s.Online = float64(m.online())
	}

	m.mu.Lock()
	m.last = now
	m.latest = s
	if len(m.ring) == metricsRing {
		copy(m.ring, m.ring[1:])
		m.ring = m.ring[:metricsRing-1]
	}
	m.ring = append(m.ring, s)
	m.mu.Unlock()
}

func (m *metrics) read() counters {
	c := counters{at: time.Now()}
	c.cpuBusy, c.cpuTotal = cpuTicks()
	c.netUp, c.netDown, c.pktUp, c.pktDown = netDev(m.dev)
	c.diskRead, c.diskWrite = diskstats()
	return c
}

func (m *metrics) Latest() sample {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.latest
}

func (m *metrics) Totals() (mem, swap float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.memTotal, m.swapTotal
}

type point struct {
	At    int64   `json:"t"`
	Value float64 `json:"v"`
}

const mostPoints = 600

func (m *metrics) Series(key string, bucket, window int) []point {
	if bucket < 1 {
		bucket = 1
	}

	m.mu.RLock()
	ring := make([]sample, len(m.ring))
	copy(ring, m.ring)
	m.mu.RUnlock()

	if window > 0 {
		since := time.Now().Unix() - int64(window)
		cut := 0
		for cut < len(ring) && ring[cut].At < since {
			cut++
		}
		ring = ring[cut:]
	}

	if want := len(ring) / mostPoints; want > bucket {
		bucket = want
	}

	out := []point{}
	var sum float64
	var count int
	var stamp int64

	for _, s := range ring {
		slot := s.At - s.At%int64(bucket)
		if count > 0 && slot != stamp {
			out = append(out, point{At: stamp, Value: sum / float64(count)})
			sum, count = 0, 0
		}
		stamp = slot
		sum += pick(s, key)
		count++
	}
	if count > 0 {
		out = append(out, point{At: stamp, Value: sum / float64(count)})
	}
	return out
}

func (m *metrics) Export() []sample {
	m.mu.RLock()
	defer m.mu.RUnlock()

	step := 1
	if len(m.ring) > mostPoints {
		step = (len(m.ring) + mostPoints - 1) / mostPoints
	}

	out := make([]sample, 0, len(m.ring)/step+1)
	for i := 0; i < len(m.ring); i += step {
		out = append(out, m.ring[i])
	}
	return out
}

func pick(s sample, key string) float64 {
	switch key {
	case "cpu":
		return s.CPU
	case "mem":
		return s.Mem
	case "swap":
		return s.Swap
	case "netUp":
		return s.NetUp
	case "netDown":
		return s.NetDown
	case "pktUp":
		return s.PktUp
	case "pktDown":
		return s.PktDown
	case "tcpCount":
		return s.TCPCount
	case "udpCount":
		return s.UDPCount
	case "diskRead":
		return s.DiskRead
	case "diskWrite":
		return s.DiskWrite
	case "diskUsage":
		return s.DiskUsage
	case "online":
		return s.Online
	case "load1":
		return s.Load1
	case "load5":
		return s.Load5
	case "load15":
		return s.Load15
	}
	return 0
}

func cpuTicks() (busy, total uint64) {
	line := firstLine("/proc/stat", "cpu ")
	for i, field := range strings.Fields(line) {
		if i == 0 {
			continue
		}
		v, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			continue
		}
		total += v
		if i != 4 && i != 5 {
			busy += v
		}
	}
	return busy, total
}

func netDev(dev string) (up, down, pktUp, pktDown uint64) {
	line := firstLine("/proc/net/dev", dev+":")
	fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), dev+":"))
	if len(fields) < 10 {
		return 0, 0, 0, 0
	}
	down, _ = strconv.ParseUint(fields[0], 10, 64)
	pktDown, _ = strconv.ParseUint(fields[1], 10, 64)
	up, _ = strconv.ParseUint(fields[8], 10, 64)
	pktUp, _ = strconv.ParseUint(fields[9], 10, 64)
	return up, down, pktUp, pktDown
}

func diskstats() (read, write uint64) {
	raw, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		name := fields[2]
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
			continue
		}
		if last := name[len(name)-1]; last >= '0' && last <= '9' && !strings.HasPrefix(name, "nvme") {
			continue
		}
		r, _ := strconv.ParseUint(fields[5], 10, 64)
		w, _ := strconv.ParseUint(fields[9], 10, 64)
		read += r * 512
		write += w * 512
	}
	return read, write
}

func meminfo(wantTotal, wantFree string) (total, free float64) {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseFloat(fields[1], 64)
		switch fields[0] {
		case wantTotal:
			total = v * 1024
		case wantFree:
			free = v * 1024
		}
	}
	return total, free
}

func sockets(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := strings.Count(string(raw), "\n")
	if n > 0 {
		n--
	}
	return n
}

func diskUsage(path string) float64 {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil || fs.Blocks == 0 {
		return 0
	}
	used := fs.Blocks - fs.Bfree
	return 100 * float64(used) / float64(fs.Blocks)
}

func diskBytes(path string) (used, total uint64) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return 0, 0
	}
	size := uint64(fs.Bsize)
	return (fs.Blocks - fs.Bfree) * size, fs.Blocks * size
}

func loadavg() (one, five, fifteen float64) {
	raw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 3 {
		return 0, 0, 0
	}
	one, _ = strconv.ParseFloat(fields[0], 64)
	five, _ = strconv.ParseFloat(fields[1], 64)
	fifteen, _ = strconv.ParseFloat(fields[2], 64)
	return one, five, fifteen
}

func hostUptime() int64 {
	raw, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return 0
	}
	secs, _ := strconv.ParseFloat(fields[0], 64)
	return int64(secs)
}

func firstLine(path, prefix string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
