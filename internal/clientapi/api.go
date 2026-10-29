package clientapi

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/qdcrypt"
	"github.com/jaywehosl/qd/internal/steerlist"
	"github.com/jaywehosl/qd/internal/update"
)

type API struct {
	db       *clientstate.DB
	platform Platform
	adblock  *Adblock

	OnImport func()
	Raise    func() error
	Lower    func() error

	mu     sync.Mutex
	netKey *qdcrypt.Key
	peers  []string

	upMu     sync.Mutex
	offer    *offerAt
	upStatus string
	upErr    string
	upDone   int64
	upTotal  int64
	fake     update.Verdict

	fakeCorrupt bool
	upFailed    string
	updatedFrom string
	updatedAt   time.Time

	failed atomic.Value
	misses atomic.Int32
}

func New(db *clientstate.DB, platform Platform, adblock *Adblock, key *qdcrypt.Key) *API {
	a := &API{db: db, platform: platform, adblock: adblock, netKey: key}
	a.noteVersion()
	return a
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
	mux.HandleFunc("/client/api/notifications/add", a.addNotification)

	mux.HandleFunc("/client/api/history/", a.history)

	mux.HandleFunc("/client/api/routing", a.routing)
	mux.HandleFunc("/client/api/routing/processes", a.processes)
	mux.HandleFunc("/client/api/routing/export", a.exportRules)
	mux.HandleFunc("/client/api/routing/import", a.importRules)

	mux.HandleFunc("/client/api/settings", a.settings)
	mux.HandleFunc("/client/api/relays", a.relays)
	mux.HandleFunc("/client/api/relays/add", a.addRelay)
	mux.HandleFunc("/client/api/relays/enable", a.enableRelay)
	mux.HandleFunc("/client/api/relays/remove", a.removeRelay)
	mux.HandleFunc("/client/api/relays/probe", a.probeRelays)
	mux.HandleFunc("/client/api/about", a.about)
	mux.HandleFunc("/client/api/update", a.updateState)
	mux.HandleFunc("/client/api/update/postpone", a.postponeUpdate)
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

	running := a.platform.Running()
	reachable := 0
	var current map[string]any
	for _, n := range nodes {
		if n.Reachable {
			reachable++
		}
		if n.Selected && running {
			current = nodeView(n)
		}
	}

	payload := map[string]any{
		"imported":  sub.Imported,
		"admin":     sub.Admin,
		"connected": running,
		"road":      a.platform.Road(),
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
	}
	if f, ok := a.platform.(interface{ Failed() bool }); ok {
		payload["failed"] = f.Failed()
	}
	return payload, nil
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

func (a *API) nodeViews() ([]map[string]any, error) {
	nodes, err := a.db.Nodes()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, nodeView(n))
	}
	return out, nil
}

func (a *API) state(w http.ResponseWriter, r *http.Request) {
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
	a.state(w, r)
}

var ErrSubscribed = errors.New("a subscription is already in use: reset the client before adding another one")

func (a *API) Import(uri string) error {
	if held, err := a.db.Subscription(); err != nil {
		return err
	} else if held.Imported {
		return ErrSubscribed
	}
	link, err := clientstate.ParseLink(uri)
	if err != nil {
		return err
	}

	nodes := make([]clientstate.Node, 0, len(link.Endpoints))
	for i, e := range link.Endpoints {
		nodes = append(nodes, clientstate.Node{
			ID: i + 1, Name: net.JoinHostPort(e.Address, strconv.Itoa(e.Port)), Role: "ingress",
			Address: e.Address, Port: e.Port, LatencyMs: -1,
		})
	}

	sub := clientstate.Subscription{
		URI: link.String(), Key: link.Key, Label: link.Label,
		Tag: link.Label, CreatedAt: time.Now().UnixMilli(),
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
	a.platform.RulesChanged()

	go a.check("added")
	return nil
}

func (a *API) check(verb string) (int, error) {
	sub, err := a.db.Subscription()
	if err != nil {
		return 0, err
	}
	if !sub.Imported {
		return 0, fmt.Errorf("nothing imported yet")
	}

	now := time.Now().UnixMilli()
	reached, err := a.take()
	if err != nil {
		return 0, err
	}
	if reached == 0 {
		a.db.Notify("warning",
			"Couldn't reach any server to update the subscription. Nothing was changed.", now)
		return 0, fmt.Errorf("no entrypoint answered")
	}

	a.db.SetLastRefresh(now)

	nodes, _ := a.db.Nodes()
	a.db.Notify("info",
		fmt.Sprintf("Subscription %s: %d of %d servers reachable.", verb, reached, len(nodes)), now)
	return reached, nil
}

func (a *API) Refresh() (int, error) { return a.check("updated") }

func (a *API) KeepFresh(stop <-chan struct{}) {
	missed := 0

	for {
		wait := min(a.untilDue(), pollCap)
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
	return min(time.Duration(missed)*30*time.Second, 10*time.Minute)
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
		a.sweep()
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
	return max(time.Until(time.UnixMilli(sub.LastRefresh).Add(every)), pollFloor)
}

func (a *API) Greet() {
	reached, err := a.take()
	if err != nil {
		fmt.Printf("access   %v\n", err)
		return
	}
	if reached > 0 {
		fmt.Printf("device   %s recognized by the network\n", a.platform.Identify().ID[:12])
	}
}

func (a *API) adoptNetworkDefaults(answer Standing) {
	if answer.Known {
		a.adoptCarriage(answer.FixedRate, answer.BBRProfile)
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
	a.db.SetAdmin(answer.Admin)
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

const peersKey = "peers"

func (a *API) Peers() []string {
	a.mu.Lock()
	held := append([]string{}, a.peers...)
	a.mu.Unlock()
	if len(held) > 0 {
		return held
	}
	text, err := a.db.Value(peersKey)
	if err != nil || text == "" {
		return nil
	}
	return strings.Split(text, ",")
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
		known[n.Endpoint()] = n
	}

	fresh := make([]clientstate.Node, 0, len(answer.Entrypoints))
	same := len(held) == len(answer.Entrypoints)
	for i, e := range answer.Entrypoints {
		if e.Address == "" || e.Port <= 0 {
			continue
		}
		node := clientstate.Node{
			ID: i + 1, Name: strings.TrimSpace(e.Name), Role: "ingress",
			Address: e.Address, Port: e.Port, LatencyMs: -1,
		}
		where := node.Endpoint()
		if node.Name == "" {
			node.Name = where
		}
		if was, carried := known[where]; carried {
			node.LatencyMs, node.Reachable, node.Selected = was.LatencyMs, was.Reachable, was.Selected
			if was.Name != node.Name {
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
		fmt.Sprintf("The list of servers changed. %d available now.", len(fresh)),
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
	if err != nil || !sub.Imported {
		return
	}
	changed := sub.AllowExit != answer.AllowExit
	if changed {
		a.db.SetAllowExit(answer.AllowExit)
	}
	if !answer.AllowExit {
		if moved, _ := a.db.DropExitRoles(); moved > 0 {
			changed = true
		}
	}
	if !changed {
		return
	}
	a.platform.RulesChanged()

	if !answer.AllowExit {
		if settings, err := a.db.Settings(); err == nil && settings.Egress {
			settings.Egress = false
			a.db.SaveSettings(settings)
			a.platform.SetExit(false)
			a.db.Notify("warning",
				"Your subscription no longer includes the exit server.",
				time.Now().UnixMilli())
		}
	}
}

func (a *API) adoptCarriage(mbit int, profile string) {
	settings, err := a.db.Settings()
	if err != nil {
		a.platform.SetCarriage(mbit, profile)
		return
	}
	a.platform.SetCarriage(mbit, profile)
	if settings.FixedRate == mbit && settings.BBRProfile == profile {
		return
	}
	settings.FixedRate, settings.BBRProfile = mbit, profile
	a.db.SaveSettings(settings)
}

func (a *API) Key() *qdcrypt.Key {
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
	bring := a.Connect
	if a.Raise != nil {
		bring = a.Raise
	}
	if err := bring(); err != nil {
		fail(w, err)
		return
	}
	a.state(w, r)
}

const quietTries = 3

func (a *API) Connect() error {
	a.misses.Store(0)
	return a.bringUp(true)
}

func (a *API) Reconnect() error { return a.bringUp(a.misses.Add(1) >= quietTries) }

func (a *API) bringUp(loud bool) error {
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

	if sub.Admin && len(a.Peers()) == 0 {
		if _, err := a.take(); err != nil {
			return err
		}
	}

	nodes, err := a.db.Nodes()
	if err != nil {
		return err
	}
	lane := Entrypoints(nodes)
	if len(lane) == 0 {
		a.db.Notify("warning", "There's no server to connect to on this network.", time.Now().UnixMilli())
		return fmt.Errorf("no entrypoint to dial")
	}

	if settings, err := a.db.Settings(); err == nil {
		a.platform.SetExit(settings.Egress && sub.AllowExit)
	}

	if err := a.platform.Start(lane, qdcrypt.SessionID(sub.Key)); err != nil {
		if prev, _ := a.failed.Load().(string); loud && prev != err.Error() && !errors.Is(err, ErrStopped) {
			a.failed.Store(err.Error())
			a.db.Notify("warning", "Couldn't connect: "+err.Error(), time.Now().UnixMilli())
		}
		return err
	}

	a.failed.Store("")
	a.misses.Store(0)
	a.db.ClearSelection()
	won := a.platform.ServerName()
	for _, n := range nodes {
		if n.Endpoint() == won {
			a.db.MarkNode(n.ID, n.LatencyMs, true, true)
			a.db.Notify("info", fmt.Sprintf("Connected via %s.", n.Name), time.Now().UnixMilli())
			break
		}
	}
	go a.Greet()
	return nil
}

func (a *API) disconnect(w http.ResponseWriter, r *http.Request) {
	drop := a.Disconnect
	if a.Lower != nil {
		drop = a.Lower
	}
	if err := drop(); err != nil {
		fail(w, err)
		return
	}
	a.state(w, r)
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
	return marshal(payload)
}

func (a *API) toggle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Egress  *bool `json:"egress"`
		Adblock *bool `json:"adblock"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Egress != nil {
		if err := a.SetEgress(*body.Egress); err != nil {
			fail(w, err)
			return
		}
	}
	if body.Adblock != nil {
		if err := a.SetAdblock(*body.Adblock); err != nil {
			fail(w, err)
			return
		}
	}
	a.state(w, r)
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
			"The exit server isn't included in your subscription.",
			time.Now().UnixMilli())
		return fmt.Errorf("this subscription does not allow exit nodes")
	}

	settings.Egress = on
	if err := a.db.SaveSettings(settings); err != nil {
		return err
	}
	a.platform.SetExit(on)
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
	a.turnAdblock(on)
	return nil
}

func (a *API) turnAdblock(on bool) {
	if a.adblock.On() == on {
		return
	}
	a.adblock.Set(on)
	if held, ok := a.platform.(interface{ FlushDNS() }); ok {
		held.FlushDNS()
	}
}

func (a *API) refresh(w http.ResponseWriter, r *http.Request) {
	reached, err := a.Refresh()
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"nodes": reached})
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
	domains, err := a.db.DomainRules()
	if err != nil {
		return "", err
	}
	return marshal(map[string]any{"defaultRole": defaultRole, "rules": rules, "domains": domains, "bundles": a.bundles(nil), "allowExit": a.exitAllowed()})
}

type bundleView struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Role     string          `json:"role"`
	Matched  int             `json:"matched"`
	Services []bundleService `json:"services"`
}

type bundleService struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (a *API) bundles(open map[string]int) []bundleView {
	roles, _ := a.db.BundleRoles()
	out := make([]bundleView, 0, len(steerlist.Bundles))
	for _, b := range steerlist.Bundles {
		role := roles[b.ID]
		if role == "" {
			role = clientstate.RoleInherit
		}
		view := bundleView{ID: b.ID, Name: b.Name, Role: role, Matched: open[steerlist.BundleMark+b.ID], Services: []bundleService{}}
		for _, s := range b.Services() {
			view.Services = append(view.Services, bundleService{ID: s.ID, Name: s.Name})
		}
		out = append(out, view)
	}
	return out
}

func (a *API) exitAllowed() bool {
	sub, err := a.db.Subscription()
	return err == nil && sub.AllowExit
}

func (a *API) SaveRulesJSON(raw string) error {
	var body struct {
		DefaultRole string                    `json:"defaultRole"`
		Rules       *[]clientstate.Rule       `json:"rules"`
		Domains     *[]clientstate.DomainRule `json:"domains"`
		Bundles     *[]struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"bundles"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return err
	}
	if body.Rules != nil || body.DefaultRole != "" {
		var rules []clientstate.Rule
		if body.Rules != nil {
			rules = *body.Rules
		} else {
			held, err := a.db.Rules()
			if err != nil {
				return err
			}
			rules = held
		}
		if body.DefaultRole == "" {
			body.DefaultRole, _ = a.db.DefaultRole()
		}
		if err := a.db.ReplaceRules(body.DefaultRole, rules); err != nil {
			return err
		}
	}
	if body.Domains != nil {
		if err := a.db.ReplaceDomainRules(*body.Domains); err != nil {
			return err
		}
	}
	if body.Bundles != nil {
		roles := map[string]string{}
		for _, b := range *body.Bundles {
			roles[b.ID] = b.Role
		}
		if err := a.db.ReplaceBundleRoles(roles); err != nil {
			return err
		}
	}
	a.platform.RulesChanged()
	return nil
}

func (a *API) SettingsJSON() (string, error) {
	settings, err := a.db.Settings()
	if err != nil {
		return "", err
	}
	return marshal(settings)
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
	next.FixedRate = current.FixedRate
	next.RefreshMinutes = min(max(next.RefreshMinutes, 1), 1440)
	switch next.RelayMode {
	case clientstate.RelayOff, clientstate.RelayAuto, clientstate.RelayOnly:
	default:
		return fmt.Errorf("%q is not a relay mode", next.RelayMode)
	}

	if next.Autostart != current.Autostart {
		if err := a.platform.HoldAutostart(next.Autostart); err != nil {
			return err
		}
	}
	if err := a.db.SaveSettings(next); err != nil {
		return err
	}
	a.platform.SetCarriage(next.FixedRate, next.BBRProfile)
	a.applyRelays()
	return nil
}

func (a *API) Reset(subscription bool) error {
	held, _ := a.db.Settings()

	if err := a.db.ResetSettings(); err != nil {
		return err
	}

	fresh, _ := a.db.Settings()
	if !subscription {
		fresh.NetworkKey, fresh.FixedRate, fresh.BBRProfile = held.NetworkKey, held.FixedRate, held.BBRProfile
		a.db.SaveSettings(fresh)
	}
	if held.Autostart != fresh.Autostart {
		a.platform.HoldAutostart(fresh.Autostart)
	}
	a.platform.SetExit(fresh.Egress)
	a.turnAdblock(fresh.Adblock)
	a.platform.RulesChanged()
	a.applyRelays()

	if !subscription {
		return nil
	}

	if a.Lower != nil {
		a.Lower()
	} else {
		a.platform.Stop()
	}
	if err := a.db.ClearSubscription(); err != nil {
		return err
	}

	a.mu.Lock()
	a.netKey, a.peers = nil, nil
	a.mu.Unlock()
	a.upMu.Lock()
	a.offer, a.upFailed, a.upErr = nil, "", ""
	a.upMu.Unlock()
	a.platform.SetKey(nil)
	return nil
}

func (a *API) MarkNoticeRead(id int) error { return a.db.MarkRead(id) }

func (a *API) AboutJSON() (string, error) {
	sub, err := a.db.Subscription()
	if err != nil {
		return "", err
	}
	up, down, _ := a.db.Traffic()

	return marshal(map[string]any{
		"version":   update.Version,
		"tag":       sub.Tag,
		"label":     sub.Label,
		"createdAt": sub.CreatedAt,
		"expiresAt": sub.ExpiresAt,
		"up":        up,
		"down":      down,
	})
}

func (a *API) NodesJSON() (string, error) {
	views, err := a.nodeViews()
	if err != nil {
		return "", err
	}
	return marshal(views)
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

func (a *API) nodes(w http.ResponseWriter, r *http.Request) {
	views, err := a.nodeViews()
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, views)
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
	a.withID(w, r, a.db.MarkRead)
}

func (a *API) dismissNotification(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, a.db.DismissNotification)
}

func (a *API) withID(w http.ResponseWriter, r *http.Request, do func(id int) error) {
	var body struct {
		ID int `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := do(body.ID); err != nil {
		fail(w, err)
		return
	}
	ok(w, nil)
}

func (a *API) addNotification(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Severity string `json:"severity"`
		Text     string `json:"text"`
	}
	if !decode(w, r, &body) {
		return
	}
	text := strings.TrimSpace(body.Text)
	if len(text) > noteLimit {
		text = text[:noteLimit]
	}
	if text == "" {
		ok(w, nil)
		return
	}
	if body.Severity != "warning" && body.Severity != "error" {
		body.Severity = "done"
	}
	now := time.Now().UnixMilli()
	if held, _, err := a.db.Notifications(); err == nil {
		for _, one := range held {
			if now-one.TS > noteEcho {
				break
			}
			if strings.Contains(one.Text, text) {
				ok(w, nil)
				return
			}
		}
	}
	if err := a.db.Notify(body.Severity, text, now); err != nil {
		fail(w, err)
		return
	}
	ok(w, nil)
}

const (
	noteLimit = 600
	noteEcho  = 5000
)

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
	switch {
	case err != nil:
		fail(w, fmt.Errorf("unknown window %s", raw))
		return
	case window != 1 && window != 5 && window != 15 && window != 60:
		fail(w, fmt.Errorf("unknown window %d", window))
		return
	}

	until := time.Now().Unix()
	points, err := a.db.Samples(until-int64(window)*60, until, 180)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"window": window, "points": points})
}

func (a *API) routing(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && !a.saveBody(w, r, a.SaveRulesJSON) {
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
	domains, err := a.db.DomainRules()
	if err != nil {
		fail(w, err)
		return
	}

	live := map[string]bool{}
	iconByPath := map[string]string{}
	iconByName := map[string]string{}
	titles := map[string]string{}
	openByName, openByPath := map[string]int{}, map[string]int{}
	for _, p := range a.platform.Processes() {
		name := strings.ToLower(p.Name)
		live[name] = true
		openByName[name] += p.Connections
		if p.Path != "" {
			openByPath[strings.ToLower(p.Path)] += p.Connections
		}
		if p.Title != "" {
			titles[name] = p.Title
		}
		if p.Icon == "" {
			continue
		}
		if p.Path != "" {
			iconByPath[strings.ToLower(p.Path)] = p.Icon
		}
		if _, held := iconByName[name]; !held {
			iconByName[name] = p.Icon
		}
	}
	onDisk, _ := a.platform.(interface{ IconOf(path string) string })
	open := map[string]int{}
	if counted, ok := a.platform.(interface{ DomainFlows() map[string]int }); ok {
		open = counted.DomainFlows()
		for i := range domains {
			domains[i].Matched = open[domains[i].Domain]
		}
	}
	for i := range rules {
		rules[i].Running = live[strings.ToLower(rules[i].Process)]
		if rules[i].Path != "" {
			rules[i].Matched = openByPath[strings.ToLower(rules[i].Path)]
		} else {
			rules[i].Matched = openByName[strings.ToLower(rules[i].Process)]
		}
		rules[i].Title = titles[strings.ToLower(rules[i].Process)]
		if icon, known := iconByPath[strings.ToLower(rules[i].Path)]; known && rules[i].Path != "" {
			rules[i].Icon = icon
			continue
		}
		if onDisk != nil && rules[i].Path != "" {
			if icon := onDisk.IconOf(rules[i].Path); icon != "" {
				rules[i].Icon = icon
				continue
			}
		}
		rules[i].Icon = iconByName[strings.ToLower(rules[i].Process)]
	}

	_, byDomain := a.platform.(interface{ RoutesByDomain() })
	ok(w, map[string]any{
		"defaultRole":    defaultRole,
		"allowExit":      a.exitAllowed(),
		"byDomain":       byDomain,
		"applyMode":      "live",
		"pendingRestart": false,
		"rules":          rules,
		"domains":        domains,
		"bundles":        a.bundles(open),
	})
}

func (a *API) processes(w http.ResponseWriter, r *http.Request) {
	ok(w, a.platform.Processes())
}

func (a *API) settings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && !a.saveBody(w, r, a.SaveSettingsJSON) {
		return
	}
	settings, err := a.db.Settings()
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, settings)
}

func (a *API) saveBody(w http.ResponseWriter, r *http.Request, save func(raw string) error) bool {
	raw, err := io.ReadAll(r.Body)
	if err == nil {
		err = save(string(raw))
	}
	if err != nil {
		fail(w, err)
		return false
	}
	return true
}

func (a *API) about(w http.ResponseWriter, r *http.Request) {
	sub, err := a.db.Subscription()
	if err != nil {
		fail(w, err)
		return
	}
	up, down, _ := a.db.Traffic()

	ok(w, map[string]any{
		"version":   update.Version,
		"tag":       sub.Tag,
		"up":        up,
		"down":      down,
		"expiresAt": sub.ExpiresAt,
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
	a.state(w, r)
}

func marshal(v any) (string, error) {
	blob, err := json.Marshal(v)
	return string(blob), err
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

func Entrypoints(nodes []clientstate.Node) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n.Role == "egress" || n.Address == "" || n.Port == 0 {
			continue
		}
		out = append(out, n.Endpoint())
	}
	return out
}
