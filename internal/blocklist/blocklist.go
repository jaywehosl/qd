package blocklist

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	origin  = "https://cdn.jsdelivr.net/gh/hagezi/dns-blocklists@latest/wildcard/"
	Threats = "tif"
	Usual   = "pro"

	fetchWait = 5 * time.Minute
	sizeCap   = 256 << 20
	roomCap   = 16 << 20
	retryIn   = 30 * time.Minute
	shortest  = time.Hour
	longest   = 7 * 24 * time.Hour
	daily     = 24 * time.Hour
)

type Source struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

var Tiers = []Source{
	{"light", "Light", origin + "light-onlydomains.txt"},
	{"normal", "Normal", origin + "multi-onlydomains.txt"},
	{"pro", "Pro", origin + "pro-onlydomains.txt"},
	{"proplus", "Pro++", origin + "pro.plus-onlydomains.txt"},
	{"ultimate", "Ultimate", origin + "ultimate-onlydomains.txt"},
}

var threats = Source{Threats, "TIF", origin + "tif-onlydomains.txt"}

func KnownTier(id string) bool {
	return slices.ContainsFunc(Tiers, func(s Source) bool { return s.ID == id })
}

func Wanted(tier string, tif bool) []Source {
	var out []Source
	for _, s := range Tiers {
		if s.ID == tier {
			out = append(out, s)
		}
	}
	if tif {
		out = append(out, threats)
	}
	return out
}

type Status struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Entries  int    `json:"entries"`
	Version  string `json:"version"`
	Modified string `json:"modified"`
	Fetched  int64  `json:"fetched"`
	Next     int64  `json:"next"`
	Loading  bool   `json:"loading"`
	Error    string `json:"error"`
}

type meta struct {
	Version  string `json:"version"`
	Modified string `json:"modified"`
	Expires  int64  `json:"expires"`
	Entries  int    `json:"entries"`
	ETag     string `json:"etag"`
	Fetched  int64  `json:"fetched"`
}

type set struct {
	meta
	hashes []uint64
}

func (s *set) period() time.Duration {
	if s.Expires <= 0 {
		return daily
	}
	return min(max(time.Duration(s.Expires)*time.Second, shortest), longest)
}

type view struct {
	sets  [][]uint64
	allow []uint64
}

type Lists struct {
	dir string
	say func(format string, args ...any)

	now  atomic.Pointer[view]
	hits atomic.Uint64
	wake chan struct{}

	mu     sync.Mutex
	want   []Source
	allow  []uint64
	held   map[string]*set
	fault  map[string]string
	retry  map[string]time.Time
	busy   map[string]bool
	forced bool
}

func New(dir string, say func(format string, args ...any)) *Lists {
	l := &Lists{
		dir: dir, say: say, wake: make(chan struct{}, 1),
		held: map[string]*set{}, fault: map[string]string{}, retry: map[string]time.Time{}, busy: map[string]bool{},
	}
	go l.keep()
	return l
}

func (l *Lists) Apply(want []Source, allow string) {
	spared := []uint64{}
	for _, line := range strings.Split(allow, "\n") {
		for _, entry := range strings.Fields(strings.ReplaceAll(cut(line), ",", " ")) {
			if name := clean(entry); name != "" {
				spared = append(spared, hashOf(name))
			}
		}
	}
	slices.Sort(spared)

	l.mu.Lock()
	l.want, l.allow = want, spared
	for _, s := range want {
		if l.held[s.ID] != nil {
			continue
		}
		if got, err := load(l.path(s.ID)); err == nil {
			l.held[s.ID] = got
		}
	}
	l.publish()
	l.mu.Unlock()
	l.nudge()
}

func (l *Lists) Refresh() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.forced = true
	l.mu.Unlock()
	l.nudge()
}

func (l *Lists) Hits() uint64 {
	if l == nil {
		return 0
	}
	return l.hits.Load()
}

func (l *Lists) Blocked(name string) bool {
	if l == nil {
		return false
	}
	v := l.now.Load()
	if v == nil || len(v.sets) == 0 {
		return false
	}

	var tails [16]uint64
	n := 0
	for rest := strings.Trim(strings.ToLower(name), "."); rest != "" && n < len(tails); n++ {
		tails[n] = hashOf(rest)
		dot := strings.IndexByte(rest, '.')
		if dot < 0 {
			rest = ""
			continue
		}
		rest = rest[dot+1:]
	}
	for _, h := range tails[:n] {
		if _, spared := slices.BinarySearch(v.allow, h); spared {
			return false
		}
	}
	for _, h := range tails[:n] {
		for _, hashes := range v.sets {
			if _, listed := slices.BinarySearch(hashes, h); listed {
				l.hits.Add(1)
				return true
			}
		}
	}
	return false
}

func (l *Lists) Status() []Status {
	out := []Status{}
	if l == nil {
		return out
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, s := range l.want {
		said := Status{ID: s.ID, Title: s.Title, Loading: l.busy[s.ID], Error: l.fault[s.ID]}
		if got := l.held[s.ID]; got != nil {
			said.Entries, said.Version, said.Modified, said.Fetched = got.Entries, got.Version, got.Modified, got.Fetched
		}
		if due := l.due(s.ID); !due.IsZero() {
			said.Next = due.Unix()
		}
		out = append(out, said)
	}
	return out
}

func (l *Lists) path(id string) string { return filepath.Join(l.dir, id+".qdl") }

func (l *Lists) nudge() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

func (l *Lists) wanted(id string) bool {
	return slices.ContainsFunc(l.want, func(s Source) bool { return s.ID == id })
}

func (l *Lists) publish() {
	arrived := true
	for _, s := range l.want {
		arrived = arrived && l.held[s.ID] != nil
	}
	v := &view{allow: l.allow}
	for id, got := range l.held {
		switch {
		case l.wanted(id) || !arrived:
			v.sets = append(v.sets, got.hashes)
		default:
			delete(l.held, id)
		}
	}
	l.now.Store(v)
}

func (l *Lists) due(id string) time.Time {
	if at, failed := l.retry[id]; failed {
		return at
	}
	got := l.held[id]
	if got == nil {
		return time.Time{}
	}
	return time.Unix(got.Fetched, 0).Add(got.period())
}

func (l *Lists) keep() {
	for {
		wait := l.round()
		select {
		case <-l.wake:
		case <-time.After(wait):
		}
	}
}

func (l *Lists) round() time.Duration {
	l.mu.Lock()
	want, forced := slices.Clone(l.want), l.forced
	l.forced = false
	l.mu.Unlock()

	next := time.Hour
	for _, s := range want {
		l.mu.Lock()
		due := l.due(s.ID)
		l.mu.Unlock()
		if forced || !time.Now().Before(due) {
			l.fetch(s)
			l.mu.Lock()
			due = l.due(s.ID)
			l.mu.Unlock()
		}
		next = min(next, time.Until(due))
	}
	return max(next, time.Minute)
}

func (l *Lists) fetch(src Source) {
	l.mu.Lock()
	was := l.held[src.ID]
	l.busy[src.ID] = true
	l.mu.Unlock()

	got, err := pull(src, was)
	if err == nil {
		err = got.save(l.path(src.ID))
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.busy, src.ID)
	if err != nil {
		l.fault[src.ID] = err.Error()
		l.retry[src.ID] = time.Now().Add(retryIn)
		l.say("lists      %s: %v, the copy in force stays", src.Title, err)
		return
	}
	delete(l.fault, src.ID)
	delete(l.retry, src.ID)
	if !l.wanted(src.ID) {
		return
	}
	l.held[src.ID] = got
	l.publish()
	if was == nil || was.Version != got.Version || len(was.hashes) != len(got.hashes) {
		l.say("lists      %s %s in force, %d names", src.Title, got.Version, len(got.hashes))
		go debug.FreeOSMemory()
	}
}

func pull(src Source, was *set) (*set, error) {
	ctx, done := context.WithTimeout(context.Background(), fetchWait)
	defer done()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, err
	}
	if was != nil && was.ETag != "" {
		req.Header.Set("If-None-Match", was.ETag)
	}
	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()

	now := time.Now().Unix()
	if rsp.StatusCode == http.StatusNotModified && was != nil {
		same := *was
		same.Fetched = now
		return &same, nil
	}
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the source answered %s", rsp.Status)
	}
	got, err := parse(io.LimitReader(rsp.Body, sizeCap))
	if err != nil {
		return nil, err
	}
	got.ETag, got.Fetched = rsp.Header.Get("ETag"), now
	return got, nil
}

func parse(r io.Reader) (*set, error) {
	got := &set{}
	declared := 0
	lines := bufio.NewScanner(r)
	lines.Buffer(make([]byte, 64<<10), 1<<20)
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		if strings.HasPrefix(line, "#") {
			key, value, ok := strings.Cut(line[1:], ":")
			if !ok {
				continue
			}
			value = strings.TrimSpace(value)
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "version":
				got.Version = value
			case "last modified":
				got.Modified = value
			case "expires":
				got.Expires = seconds(value)
			case "number of entries":
				declared, _ = strconv.Atoi(value)
				if got.hashes == nil && declared > 0 && declared < roomCap {
					got.hashes = make([]uint64, 0, declared)
				}
			}
			continue
		}
		if name := clean(line); name != "" {
			got.hashes = append(got.hashes, hashOf(name))
		}
	}
	if err := lines.Err(); err != nil {
		return nil, err
	}
	switch {
	case len(got.hashes) == 0:
		return nil, errors.New("the list came empty")
	case len(got.hashes) < declared/100*95:
		return nil, fmt.Errorf("the list came short, %d names of %d", len(got.hashes), declared)
	}
	got.Entries = len(got.hashes)
	slices.Sort(got.hashes)
	got.hashes = slices.Compact(got.hashes)
	return got, nil
}

func seconds(span string) int64 {
	fields := strings.Fields(span)
	if len(fields) < 2 {
		return 0
	}
	n, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	switch unit := strings.ToLower(fields[1]); {
	case strings.HasPrefix(unit, "hour"):
		return n * 3600
	case strings.HasPrefix(unit, "day"):
		return n * 86400
	}
	return 0
}

func cut(line string) string {
	if at := strings.IndexByte(line, '#'); at >= 0 {
		return line[:at]
	}
	return line
}

func clean(entry string) string {
	name := strings.ToLower(strings.Trim(strings.TrimPrefix(strings.TrimSpace(entry), "*."), "."))
	if !strings.Contains(name, ".") || strings.ContainsAny(name, " \t/:|^$@") {
		return ""
	}
	return name
}

func hashOf(name string) uint64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(name); i++ {
		h ^= uint64(name[i])
		h *= 1099511628211
	}
	return h
}

func (s *set) save(path string) error {
	head, err := json.Marshal(s.meta)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path + ".tmp")
	if err != nil {
		return err
	}
	out := bufio.NewWriterSize(file, 64<<10)
	out.Write(head)
	out.WriteByte('\n')
	var word [8]byte
	for _, h := range s.hashes {
		binary.LittleEndian.PutUint64(word[:], h)
		out.Write(word[:])
	}
	err = out.Flush()
	if shut := file.Close(); err == nil {
		err = shut
	}
	if err != nil {
		os.Remove(path + ".tmp")
		return err
	}
	return os.Rename(path+".tmp", path)
}

func load(path string) (*set, error) {
	damaged := errors.New("blocklist: the copy on disk is damaged")
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	in := bufio.NewReaderSize(file, 64<<10)
	head, err := in.ReadBytes('\n')
	got := &set{}
	if err != nil || json.Unmarshal(head, &got.meta) != nil {
		return nil, damaged
	}
	left := info.Size() - int64(len(head))
	if left <= 0 || left%8 != 0 {
		return nil, damaged
	}
	got.hashes = make([]uint64, left/8)
	var word [8]byte
	for i := range got.hashes {
		if _, err := io.ReadFull(in, word[:]); err != nil {
			return nil, damaged
		}
		got.hashes[i] = binary.LittleEndian.Uint64(word[:])
	}
	return got, nil
}
