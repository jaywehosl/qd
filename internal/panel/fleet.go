package panel

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
	"github.com/jaywehosl/quic-diver/internal/qwire"
)

type NodeAddress struct {
	ID      int    `json:"id"`
	Tag     string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	Role    string `json:"role"`
	Enable  bool   `json:"enable"`

	DNSPrimary   string `json:"dnsPrimary"`
	DNSSecondary string `json:"dnsSecondary"`

	Authority string `json:"authority"`
	CertPath  string `json:"certPath"`
	KeyPath   string `json:"keyPath"`
}

type NodeHealth struct {
	NodeAddress
	Online    bool    `json:"online"`
	Status    string  `json:"status"`
	LatencyMs int     `json:"latencyMs"`
	Revision  int     `json:"revision"`
	Applied   int     `json:"appliedRevision"`
	UptimeSec int64   `json:"uptimeSecs"`
	Version   string  `json:"panelVersion"`
	Heartbeat int64   `json:"lastHeartbeat"`
	CPUPct    float64 `json:"cpuPct"`
	MemPct    float64 `json:"memPct"`
	Carrying  int     `json:"carrying"`
	Error     string  `json:"lastError,omitempty"`
}

type Fleet struct {
	key   qdcrypt.Key
	token string
	tag   string
	wire  *qwire.Dialer

	mu    sync.RWMutex
	nodes map[int]NodeAddress
	seen  map[int]NodeHealth
}

func NewFleet(key qdcrypt.Key, wire *qwire.Dialer) *Fleet {
	return &Fleet{
		key:   key,
		wire:  wire,
		nodes: map[int]NodeAddress{},
		seen:  map[int]NodeHealth{},
	}
}

func (f *Fleet) Token() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.token
}

func (f *Fleet) SetToken(token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.token == token {
		return
	}
	f.token = token
}

func (f *Fleet) Tag() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.tag
}

func (f *Fleet) SetTag(tag string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tag = tag
}

func (f *Fleet) Add(addr NodeAddress) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nodes[addr.ID] = addr
}

func (f *Fleet) Nodes() []NodeAddress {
	f.mu.RLock()
	defer f.mu.RUnlock()

	out := make([]NodeAddress, 0, len(f.nodes))
	for _, n := range f.nodes {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (f *Fleet) ask(addr NodeAddress, op string, body any) (json.RawMessage, error) {
	if f.wire == nil {
		return nil, fmt.Errorf("no way to reach %s", addr.Tag)
	}
	return f.wire.Raw(net.JoinHostPort(addr.Address, strconv.Itoa(addr.Port)), op, f.Token(), body)
}

func (f *Fleet) Discover(seed NodeAddress) error {
	body, err := f.ask(seed, "nodes.list", nil)
	if err != nil {
		f.Add(seed)
		return err
	}

	var rows []NodeAddress
	if err := json.Unmarshal(body, &rows); err != nil {
		f.Add(seed)
		return err
	}

	for _, r := range rows {
		if r.Address == "" {
			continue
		}
		f.Add(r)
	}
	return nil
}

func (f *Fleet) IsAdmin(addr NodeAddress, token string) bool {
	body, err := f.ask(addr, "whoami", map[string]string{"token": token})
	if err != nil {
		return false
	}
	var answer struct {
		Admin bool `json:"admin"`
	}
	json.Unmarshal(body, &answer)
	return answer.Admin
}

func (f *Fleet) Live() []NodeAddress {
	nodes := f.Nodes()
	out := make([]NodeAddress, 0, len(nodes))
	for _, n := range nodes {
		if f.answeredLately(n.ID) {
			out = append(out, n)
		}
	}
	return out
}

func (f *Fleet) answeredLately(id int) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	seen, known := f.seen[id]
	if !known || !seen.Online || seen.Heartbeat <= 0 {
		return false
	}
	return time.Since(time.Unix(seen.Heartbeat, 0)) < liveWindow
}

const liveWindow = 90 * time.Second

func (f *Fleet) byFreshness(nodes []NodeAddress) []NodeAddress {
	f.mu.RLock()
	defer f.mu.RUnlock()

	out := append([]NodeAddress(nil), nodes...)
	sort.SliceStable(out, func(i, j int) bool {
		return f.seen[out[i].ID].Revision > f.seen[out[j].ID].Revision
	})
	return out
}

func (f *Fleet) Read(op string, body any) (json.RawMessage, error) {
	nodes := f.Nodes()
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no node to ask")
	}

	if live := f.Live(); len(live) > 0 {
		nodes = live
	}
	nodes = f.byFreshness(nodes)

	type reply struct {
		body json.RawMessage
		err  error
	}
	answers := make(chan reply, len(nodes))

	for _, n := range nodes {
		go func(n NodeAddress) {
			answer, err := f.ask(n, op, body)
			answers <- reply{body: answer, err: err}
		}(n)
	}

	var last error
	for range nodes {
		got := <-answers
		if got.err == nil {
			return got.body, nil
		}
		last = got.err
	}
	return nil, last
}

func (f *Fleet) TagOf(id int) string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.nodes[id].Tag
}

func (f *Fleet) Gather(op string, body any) map[int]json.RawMessage {
	live := f.Live()
	out := make(map[int]json.RawMessage, len(live))
	if len(live) == 0 {
		return out
	}

	type got struct {
		id   int
		body json.RawMessage
	}
	answers := make(chan got, len(live))
	for _, n := range live {
		go func(n NodeAddress) {
			answer, err := f.ask(n, op, body)
			if err != nil {
				answers <- got{id: n.ID}
				return
			}
			answers <- got{id: n.ID, body: answer}
		}(n)
	}

	deadline := time.After(gatherWait)
	for range live {
		select {
		case g := <-answers:
			if g.body != nil {
				out[g.id] = g.body
			}
		case <-deadline:
			return out
		}
	}
	return out
}

const gatherWait = 5 * time.Second

func (f *Fleet) Ask(id int, op string, body any) (json.RawMessage, error) {
	f.mu.RLock()
	node, known := f.nodes[id]
	f.mu.RUnlock()
	if !known {
		return nil, fmt.Errorf("no node %d", id)
	}
	return f.ask(node, op, body)
}

type WriteResult struct {
	NodeID     int    `json:"nodeId"`
	Tag        string `json:"tag"`
	OK         bool   `json:"ok"`
	Skipped    bool   `json:"skipped,omitempty"`
	Error      string `json:"error,omitempty"`
	Restarting bool   `json:"restarting,omitempty"`
	Moved      string `json:"moved,omitempty"`
}

func (f *Fleet) Write(op string, body any) ([]WriteResult, error) {
	return f.WriteExcept(op, body, 0)
}

func (f *Fleet) WriteExcept(op string, body any, skip int) ([]WriteResult, error) {
	nodes := f.Nodes()
	if skip != 0 {
		kept := nodes[:0]
		for _, n := range nodes {
			if n.ID != skip {
				kept = append(kept, n)
			}
		}
		nodes = kept
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no node to write to")
	}

	body = f.numbered(body)

	live := map[int]bool{}
	for _, n := range f.Live() {
		live[n.ID] = true
	}
	anyLive := false
	for _, n := range nodes {
		anyLive = anyLive || live[n.ID]
	}

	results := make([]WriteResult, len(nodes))
	var wg sync.WaitGroup

	for i, n := range nodes {
		if anyLive && !live[n.ID] {
			results[i] = WriteResult{NodeID: n.ID, Tag: n.Tag, Skipped: true,
				Error: "has not answered lately, it catches up when it is back"}
			continue
		}
		wg.Add(1)
		go func(i int, n NodeAddress) {
			defer wg.Done()
			results[i] = WriteResult{NodeID: n.ID, Tag: n.Tag, OK: true}
			said, err := f.ask(n, op, body)
			if err != nil {
				results[i].OK = false
				results[i].Error = err.Error()
				return
			}
			var took struct {
				Restarting bool   `json:"restarting"`
				Moved      string `json:"moved"`
			}
			if json.Unmarshal(said, &took) == nil && took.Restarting {
				results[i].Restarting = true
				results[i].Moved = took.Moved
			}
		}(i, n)
	}
	wg.Wait()

	took, missed := 0, ""
	for _, r := range results {
		if r.OK {
			took++
			continue
		}
		if r.Skipped {
			continue
		}
		if missed == "" {
			missed = fmt.Sprintf("%s did not take the change: %s", r.Tag, r.Error)
		}
	}
	if took == 0 {
		return results, fmt.Errorf("%s", missed)
	}
	return results, nil
}

func (f *Fleet) numbered(body any) any {
	if body == nil {
		return body
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return body
	}
	fields := map[string]any{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return body
	}
	if _, taken := fields["revision"]; taken {
		return body
	}

	f.mu.RLock()
	top := 0
	for _, h := range f.seen {
		if h.Revision > top {
			top = h.Revision
		}
	}
	f.mu.RUnlock()

	fields["revision"] = top + 1
	return fields
}

func (f *Fleet) Refresh() {
	body, err := f.Read("nodes.list", nil)
	if err != nil {
		return
	}
	var rows []NodeAddress
	if err := json.Unmarshal(body, &rows); err != nil {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range rows {
		if known, ok := f.nodes[r.ID]; ok {
			r.Address = known.Address
		}
		if r.Address == "" {
			continue
		}
		f.nodes[r.ID] = r
	}
	for id := range f.nodes {
		found := false
		for _, r := range rows {
			if r.ID == id {
				found = true
				break
			}
		}
		if !found {
			delete(f.nodes, id)
		}
	}
}

func (f *Fleet) Health() []NodeHealth {
	f.Refresh()
	nodes := f.Nodes()
	out := make([]NodeHealth, len(nodes))

	live := map[int]bool{}
	for _, n := range f.Live() {
		live[n.ID] = true
	}

	type probed struct {
		i      int
		health NodeHealth
	}
	answers := make(chan probed, len(nodes))
	awaited := make([]bool, len(nodes))
	awaiting := 0

	for i, n := range nodes {
		if len(live) == 0 || live[n.ID] {
			awaited[i] = true
			awaiting++
		}
		go func(i int, n NodeAddress) {
			health := NodeHealth{NodeAddress: n}
			started := time.Now()
			body, err := f.ask(n, "hello", nil)
			if err != nil {
				health.Error = err.Error()
				health.Status = "offline"
				answers <- probed{i, health}
				return
			}

			var info struct {
				Revision  int     `json:"revision"`
				UptimeSec int64   `json:"uptimeSecs"`
				CPUPct    float64 `json:"cpuPct"`
				MemPct    float64 `json:"memPct"`
				Carrying  int     `json:"carrying"`
				Version   string  `json:"version"`
			}
			json.Unmarshal(body, &info)

			health.Online = true
			health.Status = "online"
			health.LatencyMs = int(time.Since(started).Milliseconds())
			health.Revision = info.Revision
			health.Applied = info.Revision
			health.UptimeSec = info.UptimeSec
			health.Version = info.Version
			health.Heartbeat = time.Now().Unix()
			health.CPUPct = info.CPUPct
			health.MemPct = info.MemPct
			health.Carrying = info.Carrying
			f.mu.Lock()
			f.seen[n.ID] = health
			f.mu.Unlock()
			answers <- probed{i, health}
		}(i, n)
	}

	deadline := time.After(healthWait)
waiting:
	for awaiting > 0 {
		select {
		case p := <-answers:
			out[p.i] = p.health
			if awaited[p.i] {
				awaiting--
			}
		case <-deadline:
			break waiting
		}
	}
	for drained := false; !drained; {
		select {
		case p := <-answers:
			out[p.i] = p.health
		default:
			drained = true
		}
	}

	f.mu.Lock()
	for i, h := range out {
		if h.ID == 0 {
			late := NodeHealth{NodeAddress: nodes[i]}
			if was, known := f.seen[nodes[i].ID]; known {
				late = was
				late.NodeAddress = nodes[i]
			}
			late.Online = false
			late.Status = "offline"
			late.LatencyMs = 0
			if late.Error == "" {
				late.Error = "the node did not answer in time"
			}
			out[i] = late
			f.seen[late.ID] = late
			continue
		}
		f.seen[h.ID] = h
	}
	f.mu.Unlock()
	return out
}

func (f *Fleet) Settled(want map[string]any, wait time.Duration) (bool, string) {
	if len(want) == 0 {
		return true, ""
	}

	deadline := time.Now().Add(wait)
	var last string

	for time.Now().Before(deadline) {
		time.Sleep(settleStep)

		body, err := f.Read("network.settings", nil)
		if err != nil {
			last = err.Error()
			continue
		}
		var now map[string]any
		if json.Unmarshal(body, &now) != nil {
			last = "the node answered with something unreadable"
			continue
		}

		if odd := disagreeing(want, now); odd != "" {
			last = odd + " did not take"
			continue
		}
		return true, ""
	}

	if last == "" {
		last = "the node did not come back in time"
	}
	return false, last
}

func disagreeing(want, now map[string]any) string {
	for key, wanted := range want {
		got, carried := now[key]
		if !carried {
			return key
		}
		if fmt.Sprint(got) != fmt.Sprint(wanted) {
			return key
		}
	}
	return ""
}

const settleStep = 700 * time.Millisecond

func (f *Fleet) Seen() []NodeHealth {
	f.mu.RLock()
	defer f.mu.RUnlock()

	out := make([]NodeHealth, 0, len(f.seen))
	for _, h := range f.seen {
		out = append(out, h)
	}
	return out
}

const healthWait = 2500 * time.Millisecond
