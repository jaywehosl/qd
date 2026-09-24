package dnsproxy

import (
	"container/list"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const port = "53"

type Record struct {
	Suffix string `json:"suffix"`
	V4     string `json:"v4,omitempty"`
	V6     string `json:"v6,omitempty"`
}

type Config struct {
	Upstreams []string
	Records   []Record
	Cache     int
	MinTTL    time.Duration
	MaxTTL    time.Duration
	Stale     time.Duration
	Timeout   time.Duration
	Forward   func(query []byte) ([]byte, error)
}

type Stats struct {
	Queries   uint64 `json:"queries"`
	Hits      uint64 `json:"hits"`
	Upstream  uint64 `json:"upstream"`
	Failed    uint64 `json:"failed"`
	Records   uint64 `json:"records"`
	Refreshed uint64 `json:"refreshed"`
	Evicted   uint64 `json:"evicted"`
	Entries   int    `json:"entries"`
	Size      int    `json:"size"`
}

type counters struct {
	queries   atomic.Uint64
	hits      atomic.Uint64
	upstream  atomic.Uint64
	failed    atomic.Uint64
	records   atomic.Uint64
	refreshed atomic.Uint64
	evicted   atomic.Uint64
}

type entry struct {
	key        string
	answer     []byte
	stored     time.Time
	expires    time.Time
	refreshing bool
	elem       *list.Element
}

type rule struct {
	suffix string
	v4     net.IP
	v6     net.IP
}

type Resolver struct {
	mu        sync.Mutex
	upstreams []*upstream
	rules     []rule
	minTTL    time.Duration
	maxTTL    time.Duration
	maxSize   int
	stale     time.Duration
	timeout   time.Duration
	forwarder func(query []byte) ([]byte, error)
	cache     map[string]*entry
	lru       *list.List
	inflight  map[string]*call

	stats counters
}

func New(cfg Config) *Resolver {
	r := &Resolver{
		cache:    map[string]*entry{},
		lru:      list.New(),
		inflight: map[string]*call{},
	}
	r.Reconfigure(cfg)
	return r
}

func (r *Resolver) Reconfigure(cfg Config) {
	if cfg.Cache < 1 {
		cfg.Cache = 1
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Second
	}
	if cfg.MaxTTL <= 0 {
		cfg.MaxTTL = time.Hour
	}

	rules := make([]rule, 0, len(cfg.Records))
	for _, rec := range cfg.Records {
		suffix := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(rec.Suffix), "*."))
		if suffix == "" {
			continue
		}
		next := rule{suffix: suffix}
		if ip := net.ParseIP(strings.TrimSpace(rec.V4)); ip != nil && ip.To4() != nil {
			next.v4 = ip
		}
		if ip := net.ParseIP(strings.TrimSpace(rec.V6)); ip != nil && ip.To4() == nil {
			next.v6 = ip
		}
		if next.v4 == nil && next.v6 == nil {
			continue
		}
		rules = append(rules, next)
	}

	r.mu.Lock()
	r.minTTL, r.maxTTL, r.stale = cfg.MinTTL, cfg.MaxTTL, cfg.Stale
	r.maxSize, r.timeout, r.rules = cfg.Cache, cfg.Timeout, rules
	r.forwarder = cfg.Forward

	same := len(cfg.Upstreams) == len(r.upstreams)
	if same {
		for i, addr := range cfg.Upstreams {
			if r.upstreams[i].addr != addr {
				same = false
				break
			}
		}
	}

	var retired []*upstream
	if !same {
		retired = r.upstreams
		next := make([]*upstream, 0, len(cfg.Upstreams))
		for _, addr := range cfg.Upstreams {
			next = append(next, newUpstream(addr))
		}
		r.upstreams = next
	}
	r.evictAbove(r.maxSize)
	r.mu.Unlock()

	for _, u := range retired {
		u.Close()
	}
}

func (r *Resolver) Answer(query []byte) ([]byte, bool, error) {
	r.stats.queries.Add(1)

	name, qtype, ok := Question(query)
	if !ok {
		r.stats.failed.Add(1)
		return nil, false, errors.New("dns: malformed question")
	}

	if answer := r.fromRecords(query, name, qtype); answer != nil {
		r.stats.records.Add(1)
		return answer, true, nil
	}

	key := name + "/" + fmt.Sprint(qtype)
	if answer, found, refresh := r.lookup(key, query); found {
		r.stats.hits.Add(1)
		if refresh {
			r.stats.refreshed.Add(1)
			go r.renew(key, query)
		}
		return answer, true, nil
	}

	answer, err := r.once(key, query)
	if err != nil {
		r.stats.failed.Add(1)
		return nil, false, err
	}
	return answer, false, nil
}

type call struct {
	done   chan struct{}
	answer []byte
	err    error
}

func (r *Resolver) once(key string, query []byte) ([]byte, error) {
	r.mu.Lock()
	if c, ok := r.inflight[key]; ok {
		r.mu.Unlock()
		<-c.done
		if c.err != nil {
			return nil, c.err
		}
		out := append([]byte(nil), c.answer...)
		copy(out[0:2], query[0:2])
		return out, nil
	}
	c := &call{done: make(chan struct{})}
	r.inflight[key] = c
	r.mu.Unlock()

	answer, ttl, err := r.forward(query)
	if err == nil {
		r.stats.upstream.Add(1)
		r.store(key, answer, ttl)
	}
	c.answer, c.err = answer, err
	r.mu.Lock()
	delete(r.inflight, key)
	r.mu.Unlock()
	close(c.done)
	return answer, err
}

func (r *Resolver) renew(key string, query []byte) {
	fresh, ttl, err := r.forward(query)
	if err != nil {
		r.mu.Lock()
		if e, ok := r.cache[key]; ok {
			e.refreshing = false
		}
		r.mu.Unlock()
		return
	}
	r.store(key, fresh, ttl)
}

func (r *Resolver) forward(query []byte) ([]byte, time.Duration, error) {
	r.mu.Lock()
	ups := make([]*upstream, len(r.upstreams))
	copy(ups, r.upstreams)
	timeout, minTTL, maxTTL, via := r.timeout, r.minTTL, r.maxTTL, r.forwarder
	r.mu.Unlock()

	if via != nil {
		answer, err := via(query)
		if err != nil {
			return nil, 0, err
		}
		return answer, answerTTL(answer, minTTL, maxTTL), nil
	}
	if len(ups) == 0 {
		return nil, 0, errors.New("dns: no upstream configured")
	}

	type heard struct {
		answer []byte
		err    error
	}
	answers := make(chan heard, len(ups))

	for _, up := range ups {
		go func(u *upstream) {
			answer, err := u.exchange(query, timeout)
			answers <- heard{answer: answer, err: err}
		}(up)
	}

	var last error
	for range ups {
		got := <-answers
		if got.err == nil {
			return got.answer, answerTTL(got.answer, minTTL, maxTTL), nil
		}
		last = got.err
	}
	return nil, 0, last
}

func (r *Resolver) lookup(key string, query []byte) ([]byte, bool, bool) {
	r.mu.Lock()
	e, ok := r.cache[key]
	if !ok {
		r.mu.Unlock()
		return nil, false, false
	}

	now := time.Now()
	fresh := now.Before(e.expires)
	usable := fresh || (r.stale > 0 && now.Before(e.expires.Add(r.stale)))

	if !usable {
		r.lru.Remove(e.elem)
		delete(r.cache, key)
		r.mu.Unlock()
		return nil, false, false
	}

	r.lru.MoveToFront(e.elem)

	refresh := false
	if !fresh && !e.refreshing {
		e.refreshing = true
		refresh = true
	}

	answer := make([]byte, len(e.answer))
	copy(answer, e.answer)
	stored := e.stored
	r.mu.Unlock()

	if fresh {
		age(answer, uint32(now.Sub(stored)/time.Second))
	} else {
		age(answer, staleTTL)
	}

	copy(answer[0:2], query[0:2])
	return answer, true, refresh
}

const staleTTL = 0xFFFFFFFF

func age(msg []byte, by uint32) {
	if len(msg) < 12 {
		return
	}
	i := afterQuestions(msg)

	records := int(binary.BigEndian.Uint16(msg[6:8])) +
		int(binary.BigEndian.Uint16(msg[8:10])) +
		int(binary.BigEndian.Uint16(msg[10:12]))

	for n := 0; n < records && i+10 <= len(msg); n++ {
		i = skipName(msg, i)
		if i+10 > len(msg) {
			return
		}

		if binary.BigEndian.Uint16(msg[i:i+2]) != 41 {
			left := binary.BigEndian.Uint32(msg[i+4 : i+8])
			if by == staleTTL || by >= left {
				left = 1
			} else {
				left -= by
			}
			binary.BigEndian.PutUint32(msg[i+4:i+8], left)
		}

		rdlen := int(binary.BigEndian.Uint16(msg[i+8 : i+10]))
		i += 10 + rdlen
	}
}

func skipName(msg []byte, i int) int {
	for i < len(msg) {
		if msg[i]&0xC0 == 0xC0 {
			return i + 2
		}
		if msg[i] == 0 {
			return i + 1
		}
		i += 1 + int(msg[i])
	}
	return i
}

func (r *Resolver) store(key string, answer []byte, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	stored := make([]byte, len(answer))
	copy(stored, answer)

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if e, ok := r.cache[key]; ok {
		e.answer, e.stored, e.expires, e.refreshing = stored, now, now.Add(ttl), false
		r.lru.MoveToFront(e.elem)
		return
	}

	r.evictAbove(r.maxSize - 1)
	e := &entry{key: key, answer: stored, stored: now, expires: now.Add(ttl)}
	e.elem = r.lru.PushFront(e)
	r.cache[key] = e
}

func (r *Resolver) evictAbove(limit int) {
	for len(r.cache) > limit {
		back := r.lru.Back()
		if back == nil {
			return
		}
		r.lru.Remove(back)
		delete(r.cache, back.Value.(*entry).key)
		r.stats.evicted.Add(1)
	}
}

func (r *Resolver) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = map[string]*entry{}
	r.lru.Init()
}

func (r *Resolver) fromRecords(query []byte, name string, qtype uint16) []byte {
	if qtype != 1 && qtype != 28 {
		return nil
	}

	r.mu.Lock()
	rules := r.rules
	r.mu.Unlock()

	for _, rec := range rules {
		if name != rec.suffix && !strings.HasSuffix(name, "."+rec.suffix) {
			continue
		}
		ip := rec.v4
		if qtype == 28 {
			ip = rec.v6
		}
		if ip == nil {
			continue
		}
		return Answer(query, ip, qtype)
	}
	return nil
}

func (r *Resolver) Stats() Stats {
	r.mu.Lock()
	entries, size := len(r.cache), r.maxSize
	r.mu.Unlock()

	return Stats{
		Queries:   r.stats.queries.Load(),
		Hits:      r.stats.hits.Load(),
		Upstream:  r.stats.upstream.Load(),
		Failed:    r.stats.failed.Load(),
		Records:   r.stats.records.Load(),
		Refreshed: r.stats.refreshed.Load(),
		Evicted:   r.stats.evicted.Load(),
		Entries:   entries,
		Size:      size,
	}
}

type upstream struct {
	addr string

	mu      sync.Mutex
	conn    *net.UDPConn
	waiters map[uint16]chan []byte
	nextID  uint16
}

func newUpstream(addr string) *upstream {
	return &upstream{addr: addr, waiters: make(map[uint16]chan []byte)}
}

func (u *upstream) Close() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.conn != nil {
		u.conn.Close()
		u.conn = nil
	}
}

func (u *upstream) dial() (*net.UDPConn, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.conn != nil {
		return u.conn, nil
	}

	raddr, err := net.ResolveUDPAddr("udp", u.addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}
	u.conn = conn
	go u.readLoop(conn)
	return conn, nil
}

func (u *upstream) drop(conn *net.UDPConn) {
	u.mu.Lock()
	if u.conn == conn {
		u.conn = nil
	}
	waiters := u.waiters
	u.waiters = make(map[uint16]chan []byte)
	u.mu.Unlock()

	conn.Close()
	for _, ch := range waiters {
		close(ch)
	}
}

func (u *upstream) readLoop(conn *net.UDPConn) {
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			u.drop(conn)
			return
		}
		if n < 12 {
			continue
		}
		id := binary.BigEndian.Uint16(buf[0:2])

		u.mu.Lock()
		ch, ok := u.waiters[id]
		if ok {
			delete(u.waiters, id)
		}
		u.mu.Unlock()
		if !ok {
			continue
		}

		answer := make([]byte, n)
		copy(answer, buf[:n])
		ch <- answer
	}
}

func (u *upstream) exchange(query []byte, timeout time.Duration) ([]byte, error) {
	conn, err := u.dial()
	if err != nil {
		return nil, err
	}

	out := make([]byte, len(query))
	copy(out, query)

	ch := make(chan []byte, 1)
	u.mu.Lock()
	var id uint16
	for i := 0; i < 65536; i++ {
		u.nextID++
		if _, taken := u.waiters[u.nextID]; !taken {
			id = u.nextID
			break
		}
	}
	if id == 0 {
		u.mu.Unlock()
		return nil, errors.New("dns: no free query id")
	}
	u.waiters[id] = ch
	u.mu.Unlock()

	binary.BigEndian.PutUint16(out[0:2], id)

	if _, err := conn.Write(out); err != nil {
		u.mu.Lock()
		delete(u.waiters, id)
		u.mu.Unlock()
		u.drop(conn)
		return nil, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case answer, alive := <-ch:
		if !alive || answer == nil {
			return nil, errors.New("dns: upstream socket reset")
		}
		copy(answer[0:2], query[0:2])
		return answer, nil
	case <-timer.C:
		u.mu.Lock()
		delete(u.waiters, id)
		u.mu.Unlock()
		return nil, errors.New("dns: upstream timeout")
	}
}

func address(entry string) string {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return ""
	}

	if addr, err := netip.ParseAddr(strings.Trim(entry, "[]")); err == nil {
		return net.JoinHostPort(addr.String(), port)
	}
	if host, p, err := net.SplitHostPort(entry); err == nil && host != "" && p != "" {
		return net.JoinHostPort(host, p)
	}
	return net.JoinHostPort(entry, port)
}

func Addresses(entries ...string) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		for _, part := range strings.Split(entry, ",") {
			if addr := address(part); addr != "" {
				out = append(out, addr)
			}
		}
	}
	return out
}

func Question(msg []byte) (string, uint16, bool) {
	if len(msg) < 12 {
		return "", 0, false
	}
	if binary.BigEndian.Uint16(msg[4:6]) == 0 {
		return "", 0, false
	}

	var sb strings.Builder
	i := 12
	for i < len(msg) {
		l := int(msg[i])
		if l == 0 {
			i++
			break
		}
		if l&0xC0 != 0 || i+1+l > len(msg) {
			return "", 0, false
		}
		if sb.Len() > 0 {
			sb.WriteByte('.')
		}
		sb.Write(msg[i+1 : i+1+l])
		i += 1 + l
	}
	if i+4 > len(msg) {
		return "", 0, false
	}
	qtype := binary.BigEndian.Uint16(msg[i : i+2])
	return strings.ToLower(sb.String()), qtype, true
}

func afterQuestions(msg []byte) int {
	i := 12
	for q := int(binary.BigEndian.Uint16(msg[4:6])); q > 0 && i < len(msg); q-- {
		i = skipName(msg, i) + 4
	}
	return i
}

func answerTTL(msg []byte, lo, hi time.Duration) time.Duration {
	if len(msg) < 12 {
		return 0
	}
	if rcode := msg[3] & 0x0F; rcode != 0 && rcode != 3 {
		return 0
	}
	answers := int(binary.BigEndian.Uint16(msg[6:8]))
	if answers == 0 {
		return lo
	}

	i := afterQuestions(msg)
	best := hi
	for a := 0; a < answers && i+12 <= len(msg); a++ {
		i = skipName(msg, i)
		if i+10 > len(msg) {
			break
		}
		ttl := time.Duration(binary.BigEndian.Uint32(msg[i+4:i+8])) * time.Second
		rdlen := int(binary.BigEndian.Uint16(msg[i+8 : i+10]))
		i += 10 + rdlen

		if ttl < best {
			best = ttl
		}
	}
	return min(max(best, lo), hi)
}

func questionEnd(msg []byte) int {
	if len(msg) < 12 {
		return -1
	}
	at := 12
	for at < len(msg) {
		length := int(msg[at])
		if length == 0 {
			at++
			if at+4 > len(msg) {
				return -1
			}
			return at + 4
		}
		if length&0xC0 != 0 {
			return -1
		}
		at += length + 1
	}
	return -1
}

func shortReply(query []byte, low byte) []byte {
	end := questionEnd(query)
	if end < 0 {
		return nil
	}
	out := make([]byte, end)
	copy(out, query[:end])
	out[2] = 0x81
	out[3] = low
	binary.BigEndian.PutUint16(out[4:6], 1)
	binary.BigEndian.PutUint16(out[6:8], 0)
	binary.BigEndian.PutUint16(out[8:10], 0)
	binary.BigEndian.PutUint16(out[10:12], 0)
	return out
}

func NoData(query []byte) []byte { return shortReply(query, 0x80) }

func NXDomain(query []byte) []byte { return shortReply(query, 0x83) }

func ServFail(query []byte) []byte { return shortReply(query, 0x82) }

func Answer(query []byte, ip net.IP, qtype uint16) []byte {
	out := NoData(query)
	if out == nil {
		return nil
	}
	binary.BigEndian.PutUint16(out[6:8], 1)

	out = append(out, 0xC0, 0x0C)
	out = append(out, byte(qtype>>8), byte(qtype))
	out = append(out, 0x00, 0x01)
	out = append(out, 0x00, 0x00, 0x01, 0x2C)

	if qtype == 1 {
		v4 := ip.To4()
		if v4 == nil {
			return nil
		}
		out = append(out, 0x00, 0x04)
		return append(out, v4...)
	}
	v6 := ip.To16()
	if v6 == nil {
		return nil
	}
	out = append(out, 0x00, 0x10)
	return append(out, v6...)
}

func FirstAddr(msg []byte, qtype uint16) (net.IP, bool) {
	if len(msg) < 12 {
		return nil, false
	}
	answers := int(binary.BigEndian.Uint16(msg[6:8]))
	i := questionEnd(msg)
	if answers == 0 || i < 0 {
		return nil, false
	}

	want := 4
	if qtype == 28 {
		want = 16
	}

	for a := 0; a < answers && i+12 <= len(msg); a++ {
		i = skipName(msg, i)
		if i+10 > len(msg) {
			return nil, false
		}
		kind := binary.BigEndian.Uint16(msg[i : i+2])
		rdlen := int(binary.BigEndian.Uint16(msg[i+8 : i+10]))
		i += 10
		if i+rdlen > len(msg) {
			return nil, false
		}
		if kind == qtype && rdlen == want {
			out := make(net.IP, want)
			copy(out, msg[i:i+rdlen])
			return out, true
		}
		i += rdlen
	}
	return nil, false
}

func Addrs(msg []byte) []netip.Addr {
	if len(msg) < 12 {
		return nil
	}
	answers := int(binary.BigEndian.Uint16(msg[6:8]))
	i := questionEnd(msg)
	if i < 0 {
		return nil
	}

	var out []netip.Addr
	for a := 0; a < answers && i+12 <= len(msg); a++ {
		i = skipName(msg, i)
		if i+10 > len(msg) {
			break
		}
		kind := binary.BigEndian.Uint16(msg[i : i+2])
		rdlen := int(binary.BigEndian.Uint16(msg[i+8 : i+10]))
		i += 10
		if i+rdlen > len(msg) {
			break
		}
		if (kind == 1 && rdlen == 4) || (kind == 28 && rdlen == 16) {
			if addr, ok := netip.AddrFromSlice(msg[i : i+rdlen]); ok {
				out = append(out, addr.Unmap())
			}
		}
		i += rdlen
	}
	return out
}

func AskFor(query []byte, qtype uint16) []byte {
	end := questionEnd(query)
	if end < 4 {
		return nil
	}
	out := make([]byte, end)
	copy(out, query[:end])
	binary.BigEndian.PutUint16(out[end-4:end-2], qtype)
	return out
}
