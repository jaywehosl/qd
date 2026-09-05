package clientapi

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
)

const probeTimeout = 1500 * time.Millisecond

const ProbeTimeout = probeTimeout

func PickNode(nodes []clientstate.Node) *clientstate.Node { return pickNode(nodes) }

type API struct {
	db       *clientstate.DB
	platform Platform
	seen     *Visits

	OnImport func()

	mu       sync.Mutex
	selected int
	netKey   *qdcrypt.Key
	peers    []string
}

func New(db *clientstate.DB, platform Platform, seen *Visits, key *qdcrypt.Key) *API {
	return &API{db: db, platform: platform, seen: seen, netKey: key}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/client/api/state", a.state)
	mux.HandleFunc("/client/api/import", a.importLink)
	mux.HandleFunc("/client/api/connect", a.connect)
	mux.HandleFunc("/client/api/disconnect", a.disconnect)
	mux.HandleFunc("/client/api/toggle", a.toggle)
	mux.HandleFunc("/client/api/subscription/refresh", a.refresh)
	mux.HandleFunc("/client/api/nodes", a.nodes)

	mux.HandleFunc("/client/api/notifications", a.notifications)
	mux.HandleFunc("/client/api/notifications/read", a.markRead)
	mux.HandleFunc("/client/api/notifications/dismiss", a.dismissNotification)
	mux.HandleFunc("/client/api/notifications/clear", a.clearNotifications)

	mux.HandleFunc("/client/api/history/", a.history)

	mux.HandleFunc("/client/api/routing", a.routing)
	mux.HandleFunc("/client/api/routing/processes", a.processes)

	mux.HandleFunc("/client/api/settings", a.settings)
	mux.HandleFunc("/client/api/about", a.about)
	mux.HandleFunc("/client/api/reset", a.reset)

	return mux
}

func (a *API) statePayload() (map[string]any, error) {
	sub, err := a.db.Subscription()
	if err != nil {
		return nil, err
	}
	nodes, err := a.db.Nodes()
	if err != nil {
		return nil, err
	}
	settings, err := a.db.Settings()
	if err != nil {
		return nil, err
	}

	reachable := 0
	var current map[string]any
	for _, n := range nodes {
		if n.Reachable {
			reachable++
		}
		if n.Selected {
			current = nodeView(n)
		}
	}
	if !a.platform.Running() {
		current = nil
	}

	return map[string]any{
		"imported":  sub.Imported,
		"admin":     sub.Admin,
		"connected": a.platform.Running(),
		"node":      current,
		"nodes":     map[string]any{"total": len(nodes), "reachable": reachable},
		"egress":    settings.Egress,
		"adblock":   settings.Adblock,
		"allowExit": sub.AllowExit,
		"subscription": map[string]any{
			"lastRefresh":     sub.LastRefresh,
			"intervalMinutes": settings.RefreshMinutes,
			"expiresAt":       sub.ExpiresAt,
		},
	}, nil
}

func nodeView(n clientstate.Node) map[string]any {
	v := map[string]any{
		"id": n.ID, "name": n.Name, "role": n.Role,
		"reachable": n.Reachable, "selected": n.Selected,
	}
	if n.LatencyMs >= 0 {
		v["latencyMs"] = n.LatencyMs
	}
	return v
}

func (a *API) state(w http.ResponseWriter, r *http.Request) {
	payload, err := a.statePayload()
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, payload)
}

func (a *API) replyState(w http.ResponseWriter) {
	payload, err := a.statePayload()
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, payload)
}

func (a *API) importLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URI string `json:"uri"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := a.Import(body.URI); err != nil {
		fail(w, err)
		return
	}
	a.replyState(w)
}

func (a *API) Import(uri string) error {
	link, err := clientstate.ParseLink(uri)
	if err != nil {
		return err
	}

	now := time.Now().UnixMilli()
	nodes := make([]clientstate.Node, 0, len(link.Endpoints))
	for i, e := range link.Endpoints {
		nodes = append(nodes, clientstate.Node{
			ID: i + 1, Name: net.JoinHostPort(e.Address, strconv.Itoa(e.Port)), Role: "ingress",
			Address: e.Address, Port: e.Port, LatencyMs: -1,
		})
	}

	sub := clientstate.Subscription{
		URI: link.String(), Key: link.Key, Label: link.Label,
		Tag: link.Label, CreatedAt: now,
	}
	if err := a.db.SaveSubscription(sub); err != nil {
		return err
	}
	if err := a.db.ReplaceNodes(nodes); err != nil {
		return err
	}

	if link.NetworkKey != "" {
		if err := a.adoptNetworkKey(link.NetworkKey); err != nil {
			return err
		}
	}

	if a.OnImport != nil {
		a.OnImport()
	}

	reached := a.ProbeAll()
	if reached == 0 {
		a.db.Notify("warning", "No node answered this link yet.", now)
		return nil
	}

	if fresh, err := a.db.Subscription(); err == nil {
		fresh.LastRefresh = time.Now().UnixMilli()
		a.db.SaveSubscription(fresh)
	}
	a.db.Notify("info", fmt.Sprintf("Subscription imported: %d of %d entrypoints reachable.", reached, len(nodes)), now)
	return nil
}

func (a *API) ProbeReach() int {
	nodes, err := a.db.Nodes()
	if err != nil {
		return 0
	}
	sub, err := a.db.Subscription()
	if err != nil || !sub.Imported {
		return 0
	}
	wire := a.wire()
	if wire == nil {
		return 0
	}

	type result struct {
		id      int
		latency int
	}
	results := make(chan result, len(nodes))

	var wg sync.WaitGroup
	for _, n := range nodes {
		wg.Add(1)
		go func(n clientstate.Node) {
			defer wg.Done()

			where := net.JoinHostPort(n.Address, strconv.Itoa(n.Port))
			began := time.Now()
			if err := wire.Ask(where, "whoami", sub.Key, claim(a.platform.Identify(), sub.Key), nil); err != nil {
				results <- result{id: n.ID, latency: -1}
				return
			}
			results <- result{id: n.ID, latency: int(time.Since(began).Milliseconds())}
		}(n)
	}
	wg.Wait()
	close(results)

	reached := 0
	for r := range results {
		if r.latency >= 0 {
			reached++
		}
		a.db.MarkReach(r.id, r.latency, r.latency >= 0)
	}
	return reached
}

func (a *API) ProbeAll() int {
	reached := a.ProbeReach()

	nodes, err := a.db.Nodes()
	if err != nil {
		return reached
	}
	sub, err := a.db.Subscription()
	if err != nil || !sub.Imported {
		return reached
	}

	if answer, heard := AskStanding(nodes, a.key(), sub.Key, a.platform.Identify(), a.wire()); heard {
		a.adoptNetworkDefaults(answer)
		if answer.Refused() {
			a.db.Notify("error", answer.Why(), time.Now().UnixMilli())
			a.platform.Stop()
		}
	}
	return reached
}

func (a *API) KeepFresh(stop <-chan struct{}) {
	missed := 0

	for {
		wait := a.untilDue()
		if wait > pollCap {
			wait = pollCap
		}
		if missed > 0 {
			wait = retryIn(missed)
		}

		select {
		case <-stop:
			return
		case <-time.After(wait):
		}

		if sub, err := a.db.Subscription(); err != nil || !sub.Imported {
			missed = 0
			continue
		}
		if a.untilDue() > pollFloor {
			continue
		}

		reached, err := a.Refresh()
		if err != nil {
			missed++
			fmt.Printf("sub      check failed: %v\n", err)
			continue
		}
		missed = 0
		fmt.Printf("sub      checked, %d entrypoints answered, next in %s\n",
			reached, a.untilDue().Round(time.Second))
	}
}

const (
	pollCap   = 20 * time.Second
	pollFloor = 5 * time.Second
)

func retryIn(missed int) time.Duration {
	wait := time.Duration(missed) * 30 * time.Second
	if wait > 10*time.Minute {
		return 10 * time.Minute
	}
	return wait
}

func (a *API) KeepProbing(stop <-chan struct{}, every time.Duration) {
	tick := time.NewTicker(every)
	defer tick.Stop()

	for {
		select {
		case <-stop:
			return
		case <-tick.C:
		}
		if sub, err := a.db.Subscription(); err != nil || !sub.Imported {
			continue
		}
		a.ProbeReach()
	}
}

func (a *API) untilDue() time.Duration {
	every := 60 * time.Minute
	if settings, err := a.db.Settings(); err == nil && settings.RefreshMinutes > 0 {
		every = time.Duration(settings.RefreshMinutes) * time.Minute
	}

	sub, err := a.db.Subscription()
	if err != nil {
		return every
	}
	if sub.LastRefresh <= 0 {
		return pollFloor
	}

	left := time.Until(time.UnixMilli(sub.LastRefresh).Add(every))
	if left < 5*time.Second {
		return 5 * time.Second
	}
	return left
}

func (a *API) Greet() {
	answer, heard := a.Standing()
	if !heard {
		return
	}
	a.adoptNetworkDefaults(answer)
	if answer.Refused() {
		a.db.Notify("error", answer.Why(), time.Now().UnixMilli())
		a.platform.Stop()
		fmt.Printf("access   %s\n", answer.Why())
		return
	}
	fmt.Printf("device   %s recognised by the network\n", a.platform.Identify().ID[:12])
}

func (a *API) adoptNetworkDefaults(answer Standing) {
	if answer.Known {
		a.adoptFixedRate(answer.FixedRate)
	}
	a.adoptExit(answer)
	a.adoptRefresh(answer)
	a.adoptEntrypoints(answer)
	a.adoptPeers(answer)
	a.adoptAdmin(answer)
}

func (a *API) adoptAdmin(answer Standing) {
	if !answer.Known {
		return
	}
	sub, err := a.db.Subscription()
	if err != nil || !sub.Imported || sub.Admin == answer.Admin {
		return
	}
	sub.Admin = answer.Admin
	a.db.SaveSubscription(sub)
}

func (a *API) adoptPeers(answer Standing) {
	if len(answer.Peers) == 0 {
		return
	}
	a.mu.Lock()
	a.peers = append([]string{}, answer.Peers...)
	a.mu.Unlock()
	a.db.SetValue(peersKey, strings.Join(answer.Peers, ","))
}

func (a *API) knownPeers() []string {
	text, err := a.db.Value(peersKey)
	if err != nil || text == "" {
		return nil
	}
	return strings.Split(text, ",")
}

const peersKey = "peers"

func (a *API) Peers() []string {
	a.mu.Lock()
	held := append([]string{}, a.peers...)
	a.mu.Unlock()
	if len(held) > 0 {
		return held
	}
	return a.knownPeers()
}

func (a *API) adoptEntrypoints(answer Standing) {
	if !answer.Known || !answer.Enable || len(answer.Entrypoints) == 0 {
		return
	}

	held, err := a.db.Nodes()
	if err != nil {
		return
	}
	known := make(map[string]clientstate.Node, len(held))
	for _, n := range held {
		known[fmt.Sprintf("%s:%d", n.Address, n.Port)] = n
	}

	fresh := make([]clientstate.Node, 0, len(answer.Entrypoints))
	same := len(held) == len(answer.Entrypoints)
	for i, e := range answer.Entrypoints {
		if e.Address == "" || e.Port <= 0 {
			continue
		}
		where := fmt.Sprintf("%s:%d", e.Address, e.Port)
		called := strings.TrimSpace(e.Name)
		if called == "" {
			called = where
		}

		node := clientstate.Node{
			ID: i + 1, Name: called, Role: "ingress",
			Address: e.Address, Port: e.Port, LatencyMs: -1,
		}
		if was, carried := known[where]; carried {
			node.LatencyMs, node.Reachable, node.Selected = was.LatencyMs, was.Reachable, was.Selected
			if was.Name != called {
				same = false
			}
		} else {
			same = false
		}
		fresh = append(fresh, node)
	}
	if len(fresh) == 0 || same {
		return
	}

	if err := a.db.ReplaceNodes(fresh); err != nil {
		return
	}
	a.db.Notify("info",
		fmt.Sprintf("The network changed the entrypoints on offer: %d now.", len(fresh)),
		time.Now().UnixMilli())
}

func (a *API) adoptRefresh(answer Standing) {
	if answer.RefreshMinutes < 1 {
		return
	}
	settings, err := a.db.Settings()
	if err != nil || settings.RefreshPinned || settings.RefreshMinutes == answer.RefreshMinutes {
		return
	}
	settings.RefreshMinutes = answer.RefreshMinutes
	a.db.SaveSettings(settings)
}

func (a *API) adoptExit(answer Standing) {
	if !answer.Known {
		return
	}
	sub, err := a.db.Subscription()
	if err != nil || !sub.Imported || sub.AllowExit == answer.AllowExit {
		return
	}
	sub.AllowExit = answer.AllowExit
	a.db.SaveSubscription(sub)

	if !answer.AllowExit {
		if settings, err := a.db.Settings(); err == nil && settings.Egress {
			settings.Egress = false
			a.db.SaveSettings(settings)
			a.platform.SetExit(false)
			a.db.Notify("warning",
				"Exit nodes are no longer allowed for this subscription.",
				time.Now().UnixMilli())
		}
	}
}

func (a *API) Standing() (Standing, bool) {
	sub, err := a.db.Subscription()
	if err != nil || !sub.Imported {
		return Standing{}, false
	}
	nodes, err := a.db.Nodes()
	if err != nil {
		return Standing{}, false
	}
	return AskStanding(nodes, a.key(), sub.Key, a.platform.Identify(), a.wire())
}

func (a *API) wire() Asker { return a.platform.Wire() }

func (a *API) Key() *qdcrypt.Key { return a.key() }

func (a *API) key() *qdcrypt.Key {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.netKey
}

func (a *API) adoptNetworkKey(text string) error {
	raw, err := hex.DecodeString(text)
	if err != nil || len(raw) != qdcrypt.KeySize {
		return fmt.Errorf("the link carries a malformed network key")
	}

	var k qdcrypt.Key
	copy(k[:], raw)

	settings, err := a.db.Settings()
	if err != nil {
		return err
	}
	settings.NetworkKey = text
	if err := a.db.SaveSettings(settings); err != nil {
		return err
	}

	a.mu.Lock()
	a.netKey = &k
	a.mu.Unlock()

	a.platform.SetKey(&k)
	return nil
}

func (a *API) connect(w http.ResponseWriter, r *http.Request) {
	if a.platform.Running() {
		a.replyState(w)
		return
	}
	if err := a.Connect(); err != nil {
		fail(w, err)
		return
	}
	a.replyState(w)
}

func (a *API) Connect() error {
	if a.platform.Running() {
		return nil
	}

	sub, err := a.db.Subscription()
	if err != nil {
		return err
	}
	if !sub.Imported {
		return fmt.Errorf("nothing imported yet")
	}

	nodes, err := a.db.Nodes()
	if err != nil {
		return err
	}

	if sub.Admin && len(a.Peers()) == 0 {
		if answer, heard := AskStanding(nodes, a.key(), sub.Key,
			a.platform.Identify(), a.wire()); heard {
			a.adoptNetworkDefaults(answer)
			if fresh, err := a.db.Nodes(); err == nil && len(fresh) > 0 {
				nodes = fresh
			}
		}
	}

	lane := Entrypoints(nodes)
	if len(lane) == 0 {
		a.db.Notify("warning", "No entrypoint to dial on this network.", time.Now().UnixMilli())
		return fmt.Errorf("no entrypoint to dial")
	}

	session := clientstate.SessionID(sub.Key)
	if settings, err := a.db.Settings(); err == nil {
		a.platform.SetExit(settings.Egress && sub.AllowExit)
	}

	if err := a.platform.Start(lane, session); err != nil {
		a.db.Notify("warning", "Could not bring the tunnel up: "+err.Error(), time.Now().UnixMilli())
		return err
	}

	a.db.ClearSelection()
	winner := whoWon(nodes, a.platform.ServerName())
	if winner != nil {
		a.db.MarkNode(winner.ID, winner.LatencyMs, true, true)
		a.db.Notify("info", fmt.Sprintf("Connected through %s.", winner.Name), time.Now().UnixMilli())
	}

	a.mu.Lock()
	if winner != nil {
		a.selected = winner.ID
	}
	a.mu.Unlock()
	return nil
}

func (a *API) Disconnect() error {
	if err := a.platform.Stop(); err != nil {
		return err
	}
	a.db.ClearSelection()
	a.db.Notify("info", "Disconnected.", time.Now().UnixMilli())
	return nil
}

func (a *API) StateJSON() (string, error) {
	payload, err := a.statePayload()
	if err != nil {
		return "", err
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(blob), nil
}

func (a *API) SetEgress(on bool) error {
	settings, err := a.db.Settings()
	if err != nil {
		return err
	}
	sub, err := a.db.Subscription()
	if err != nil {
		return err
	}
	if on && !sub.AllowExit {
		a.db.Notify("warning",
			"Exit nodes were refused: this subscription's group does not allow them.",
			time.Now().UnixMilli())
		return fmt.Errorf("this subscription does not allow exit nodes")
	}

	settings.Egress = on
	if err := a.db.SaveSettings(settings); err != nil {
		return err
	}
	a.platform.SetExit(on && sub.AllowExit)
	return nil
}

func (a *API) SetAdblock(on bool) error {
	settings, err := a.db.Settings()
	if err != nil {
		return err
	}
	settings.Adblock = on
	if err := a.db.SaveSettings(settings); err != nil {
		return err
	}
	a.seen.SetAdblock(on)
	return nil
}

func (a *API) Refresh() (int, error) {
	sub, err := a.db.Subscription()
	if err != nil {
		return 0, err
	}
	if !sub.Imported {
		return 0, fmt.Errorf("nothing imported yet")
	}

	now := time.Now().UnixMilli()

	if answer, heard := a.Standing(); heard {
		a.adoptNetworkDefaults(answer)
		if answer.Refused() {
			a.platform.Stop()
			a.db.Notify("error", answer.Why(), now)
			return 0, fmt.Errorf("%s", answer.Why())
		}
	}

	reached := a.ProbeAll()
	nodes, _ := a.db.Nodes()

	if reached == 0 {
		a.db.Notify("warning",
			"No entrypoint answered — the subscription was left as it stands.", now)
		return 0, fmt.Errorf("no entrypoint answered")
	}

	if fresh, err := a.db.Subscription(); err == nil {
		sub = fresh
	}
	sub.LastRefresh = now
	a.db.SaveSubscription(sub)

	a.db.Notify("info",
		fmt.Sprintf("Subscription checked: %d of %d entrypoints reachable.", reached, len(nodes)), now)
	return reached, nil
}

func (a *API) RulesJSON() (string, error) {
	rules, err := a.db.Rules()
	if err != nil {
		return "", err
	}
	defaultRole, err := a.db.DefaultRole()
	if err != nil {
		return "", err
	}

	blob, err := json.Marshal(map[string]any{
		"defaultRole": defaultRole,
		"rules":       rules,
	})
	if err != nil {
		return "", err
	}
	return string(blob), nil
}

func (a *API) SaveRulesJSON(raw string) error {
	var body struct {
		DefaultRole string             `json:"defaultRole"`
		Rules       []clientstate.Rule `json:"rules"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return err
	}
	if err := a.db.ReplaceRules(body.DefaultRole, body.Rules); err != nil {
		return err
	}
	a.platform.RulesChanged()
	return nil
}

func (a *API) SettingsJSON() (string, error) {
	settings, err := a.db.Settings()
	if err != nil {
		return "", err
	}
	blob, err := json.Marshal(settings)
	if err != nil {
		return "", err
	}
	return string(blob), nil
}

func (a *API) SaveSettingsJSON(raw string) error {
	current, err := a.db.Settings()
	if err != nil {
		return err
	}

	next := current
	if err := json.Unmarshal([]byte(raw), &next); err != nil {
		return err
	}

	if next.RefreshMinutes != current.RefreshMinutes {
		next.RefreshPinned = true
	}
	if next.FixedRate != current.FixedRate {
		next.RatePinned = true
	}
	if next.FixedRate < 0 {
		next.FixedRate = 0
	}
	if next.FixedRate > 10000 {
		next.FixedRate = 10000
	}
	if next.RefreshMinutes < 1 {
		next.RefreshMinutes = 1
	}
	if next.RefreshMinutes > 1440 {
		next.RefreshMinutes = 1440
	}
	next.Egress = current.Egress
	next.Adblock = current.Adblock
	next.NetworkKey = current.NetworkKey
	if err := a.db.SaveSettings(next); err != nil {
		return err
	}
	a.platform.SetFixedRate(next.FixedRate)
	return nil
}

func (a *API) Reset(subscription bool) error {
	held, _ := a.db.Settings()

	if err := a.db.ResetSettings(); err != nil {
		return err
	}

	if !subscription {
		if held.NetworkKey != "" {
			if fresh, err := a.db.Settings(); err == nil {
				fresh.NetworkKey = held.NetworkKey
				a.db.SaveSettings(fresh)
			}
		}
		return nil
	}

	a.platform.Stop()
	if err := a.db.ClearSubscription(); err != nil {
		return err
	}

	a.mu.Lock()
	a.netKey = nil
	a.mu.Unlock()
	a.platform.SetKey(nil)
	return nil
}

func (a *API) UnreadJSON() (string, error) {
	items, _, err := a.db.Notifications()
	if err != nil {
		return "", err
	}

	fresh := []clientstate.Notification{}
	for _, item := range items {
		if !item.Read {
			fresh = append(fresh, item)
		}
	}
	if len(fresh) > 1 {
		for i, j := 0, len(fresh)-1; i < j; i, j = i+1, j-1 {
			fresh[i], fresh[j] = fresh[j], fresh[i]
		}
	}

	blob, err := json.Marshal(fresh)
	if err != nil {
		return "", err
	}
	return string(blob), nil
}

func (a *API) MarkNoticeRead(id int) error { return a.db.MarkRead(id) }

func (a *API) AboutJSON() (string, error) {
	sub, err := a.db.Subscription()
	if err != nil {
		return "", err
	}
	up, down, _ := a.db.Traffic()

	blob, err := json.Marshal(map[string]any{
		"tag":       sub.Tag,
		"label":     sub.Label,
		"createdAt": sub.CreatedAt,
		"expiresAt": sub.ExpiresAt,
		"up":        up,
		"down":      down,
	})
	if err != nil {
		return "", err
	}
	return string(blob), nil
}

func (a *API) NodesJSON() (string, error) {
	nodes, err := a.db.Nodes()
	if err != nil {
		return "", err
	}
	out := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, nodeView(n))
	}
	blob, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(blob), nil
}

func (a *API) Selected() (clientstate.Node, bool) {
	nodes, err := a.db.Nodes()
	if err != nil {
		return clientstate.Node{}, false
	}
	for _, n := range nodes {
		if n.Selected {
			return n, true
		}
	}
	return clientstate.Node{}, false
}

func pickNode(nodes []clientstate.Node) *clientstate.Node {
	var best *clientstate.Node
	for i := range nodes {
		n := &nodes[i]
		if n.Role == "egress" {
			continue
		}
		if best == nil {
			best = n
			continue
		}
		if better(n, best) {
			best = n
		}
	}
	return best
}

func better(a, b *clientstate.Node) bool {
	if a.Reachable != b.Reachable {
		return a.Reachable
	}
	if a.LatencyMs < 0 {
		return false
	}
	if b.LatencyMs < 0 {
		return true
	}
	return a.LatencyMs < b.LatencyMs
}

func (a *API) disconnect(w http.ResponseWriter, r *http.Request) {
	if err := a.platform.Stop(); err != nil {
		fail(w, err)
		return
	}
	a.db.ClearSelection()
	a.db.Notify("info", "Disconnected.", time.Now().UnixMilli())
	a.replyState(w)
}

func (a *API) toggle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Egress  *bool `json:"egress"`
		Adblock *bool `json:"adblock"`
	}
	if !decode(w, r, &body) {
		return
	}

	settings, err := a.db.Settings()
	if err != nil {
		fail(w, err)
		return
	}
	sub, err := a.db.Subscription()
	if err != nil {
		fail(w, err)
		return
	}

	if body.Egress != nil {
		if *body.Egress && !sub.AllowExit {
			a.db.Notify("warning",
				"Exit nodes were refused: this subscription's group does not allow them.",
				time.Now().UnixMilli())
			fail(w, fmt.Errorf("this subscription does not allow exit nodes"))
			return
		}
		settings.Egress = *body.Egress
	}
	if body.Adblock != nil {
		settings.Adblock = *body.Adblock
	}

	if err := a.db.SaveSettings(settings); err != nil {
		fail(w, err)
		return
	}
	a.seen.SetAdblock(settings.Adblock)
	a.platform.SetExit(settings.Egress && sub.AllowExit)
	a.replyState(w)
}

func (a *API) refresh(w http.ResponseWriter, r *http.Request) {
	sub, err := a.db.Subscription()
	if err != nil {
		fail(w, err)
		return
	}
	if !sub.Imported {
		fail(w, fmt.Errorf("nothing imported yet"))
		return
	}

	reached := a.ProbeAll()
	now := time.Now().UnixMilli()

	if answer, heard := a.Standing(); heard {
		a.adoptNetworkDefaults(answer)
		if answer.Refused() {
			a.platform.Stop()
			a.db.Notify("error", answer.Why(), now)
			fail(w, fmt.Errorf("%s", answer.Why()))
			return
		}
	}

	nodes, _ := a.db.Nodes()
	if reached == 0 {
		a.db.Notify("warning",
			"No entrypoint answered — the subscription was left as it stands.", now)
		fail(w, fmt.Errorf("no entrypoint answered"))
		return
	}

	if fresh, err := a.db.Subscription(); err == nil {
		sub = fresh
	}
	sub.LastRefresh = now
	a.db.SaveSubscription(sub)

	a.db.Notify("info",
		fmt.Sprintf("Subscription checked: %d of %d entrypoints reachable.", reached, len(nodes)), now)

	ok(w, map[string]any{"changed": false, "nodes": reached, "reconnected": false})
}

func (a *API) nodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := a.db.Nodes()
	if err != nil {
		fail(w, err)
		return
	}
	out := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, nodeView(n))
	}
	ok(w, out)
}

func (a *API) notifications(w http.ResponseWriter, r *http.Request) {
	items, unread, err := a.db.Notifications()
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"unread": unread, "items": items})
}

func (a *API) markRead(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID int `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := a.db.MarkRead(body.ID); err != nil {
		fail(w, err)
		return
	}
	ok(w, nil)
}

func (a *API) dismissNotification(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID int `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := a.db.DismissNotification(body.ID); err != nil {
		fail(w, err)
		return
	}
	ok(w, nil)
}

func (a *API) clearNotifications(w http.ResponseWriter, r *http.Request) {
	if err := a.db.ClearNotifications(); err != nil {
		fail(w, err)
		return
	}
	ok(w, nil)
}

func (a *API) history(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimPrefix(r.URL.Path, "/client/api/history/")
	window, err := strconv.Atoi(raw)
	if err != nil {
		fail(w, fmt.Errorf("unknown window %s", raw))
		return
	}
	switch window {
	case 1, 5, 15, 60:
	default:
		fail(w, fmt.Errorf("unknown window %d", window))
		return
	}

	until := time.Now().Unix()
	since := until - int64(window)*60
	points, err := a.db.Samples(since, until, 180)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"window": window, "points": points})
}

func (a *API) routing(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		a.saveRouting(w, r)
		return
	}

	rules, err := a.db.Rules()
	if err != nil {
		fail(w, err)
		return
	}
	defaultRole, err := a.db.DefaultRole()
	if err != nil {
		fail(w, err)
		return
	}

	running := a.platform.Processes()
	live := map[string]bool{}
	iconByPath := map[string]string{}
	iconByName := map[string]string{}
	for _, p := range running {
		live[strings.ToLower(p.Name)] = true
		if p.Icon == "" {
			continue
		}
		if p.Path != "" {
			iconByPath[strings.ToLower(p.Path)] = p.Icon
		}
		if _, held := iconByName[strings.ToLower(p.Name)]; !held {
			iconByName[strings.ToLower(p.Name)] = p.Icon
		}
	}
	for i := range rules {
		rules[i].Running = live[strings.ToLower(rules[i].Process)]
		if icon, known := iconByPath[strings.ToLower(rules[i].Path)]; known && rules[i].Path != "" {
			rules[i].Icon = icon
			continue
		}
		rules[i].Icon = iconByName[strings.ToLower(rules[i].Process)]
	}

	ok(w, map[string]any{
		"defaultRole":    defaultRole,
		"applyMode":      "live",
		"pendingRestart": false,
		"rules":          rules,
	})
}

func (a *API) saveRouting(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DefaultRole string             `json:"defaultRole"`
		Rules       []clientstate.Rule `json:"rules"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := a.db.ReplaceRules(body.DefaultRole, body.Rules); err != nil {
		fail(w, err)
		return
	}
	a.platform.RulesChanged()
	a.routing(w, &http.Request{Method: http.MethodGet, URL: r.URL})
}

func (a *API) processes(w http.ResponseWriter, r *http.Request) {
	ok(w, a.platform.Processes())
}

func (a *API) settings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var body clientstate.Settings
		current, err := a.db.Settings()
		if err != nil {
			fail(w, err)
			return
		}
		body = current
		if !decode(w, r, &body) {
			return
		}
		if body.RefreshMinutes != current.RefreshMinutes {
			body.RefreshPinned = true
		}
		if body.RefreshMinutes < 1 {
			body.RefreshMinutes = 1
		}
		if body.RefreshMinutes > 1440 {
			body.RefreshMinutes = 1440
		}
		body.Egress = current.Egress
		body.Adblock = current.Adblock
		if body.Autostart != current.Autostart {
			if err := a.platform.HoldAutostart(body.Autostart); err != nil {
				fail(w, err)
				return
			}
		}
		if err := a.db.SaveSettings(body); err != nil {
			fail(w, err)
			return
		}
	}

	settings, err := a.db.Settings()
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, settings)
}

func (a *API) about(w http.ResponseWriter, r *http.Request) {
	sub, err := a.db.Subscription()
	if err != nil {
		fail(w, err)
		return
	}
	up, down, _ := a.db.Traffic()
	sites, _ := a.db.TopSites(10)

	ok(w, map[string]any{
		"tag":       sub.Tag,
		"createdAt": sub.CreatedAt,
		"up":        up,
		"down":      down,
		"expiresAt": sub.ExpiresAt,
		"topSites":  sites,
	})
}

func (a *API) reset(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Subscription bool `json:"subscription"`
	}
	if !decode(w, r, &body) {
		return
	}

	if err := a.Reset(body.Subscription); err != nil {
		fail(w, err)
		return
	}
	a.replyState(w)
}

func ok(w http.ResponseWriter, obj any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true, "msg": "", "obj": obj})
}

func fail(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": false, "msg": err.Error(), "obj": nil})
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		fail(w, fmt.Errorf("bad request body: %w", err))
		return false
	}
	return true
}

func (a *API) adoptFixedRate(mbit int) {
	settings, err := a.db.Settings()
	if err != nil {
		a.platform.SetFixedRate(mbit)
		return
	}
	if settings.RatePinned {
		a.platform.SetFixedRate(settings.FixedRate)
		return
	}

	a.platform.SetFixedRate(mbit)
	if settings.FixedRate == mbit {
		return
	}
	settings.FixedRate = mbit
	a.db.SaveSettings(settings)
}

func Entrypoints(nodes []clientstate.Node) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n.Role == "egress" || n.Address == "" || n.Port == 0 {
			continue
		}
		out = append(out, net.JoinHostPort(n.Address, strconv.Itoa(n.Port)))
	}
	return out
}

func whoWon(nodes []clientstate.Node, endpoint string) *clientstate.Node {
	for i := range nodes {
		if net.JoinHostPort(nodes[i].Address, strconv.Itoa(nodes[i].Port)) == endpoint {
			return &nodes[i]
		}
	}
	return nil
}
