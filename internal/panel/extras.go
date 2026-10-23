package panel

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/jaywehosl/qd/internal/qdcrypt"
)

const prefsKey = "panel.prefs"

func freshSecret() string {
	raw := make([]byte, 32)
	rand.Read(raw)
	return hex.EncodeToString(raw)
}

func (a *API) extraRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/panel/api/server/status", a.serverStatus)
	mux.HandleFunc("/panel/api/entrypoints", a.entrypointsList)

	mux.HandleFunc("/panel/api/clients/onlines", a.onlines)
	mux.HandleFunc("/panel/api/clients/lastOnline", a.lastOnline)

	mux.HandleFunc("/panel/setting/all", a.settings)
	mux.HandleFunc("/panel/setting/update", a.settings)

	mux.HandleFunc("/panel/api/releases", a.releases)
	a.dnsRoutes(mux)
	a.themeRoutes(mux)
}

func (a *API) serverStatus(w http.ResponseWriter, r *http.Request) {
	health := a.fleet.Health()
	online := 0
	first := 0
	for _, h := range health {
		if h.Online {
			online++
			if first == 0 {
				first = h.ID
			}
		}
	}

	status := map[string]any{
		"appStats":    map[string]any{"mem": 0, "threads": runtime.NumGoroutine(), "uptime": 0},
		"appUptime":   0,
		"cpu":         0,
		"cpuCores":    runtime.NumCPU(),
		"cpuSpeedMhz": 0,
		"logicalPro":  runtime.NumCPU(),
		"loads":       []float64{0, 0, 0},
		"disk":        map[string]any{"current": 0, "total": 0},
		"mem":         map[string]any{"current": 0, "total": 0},
		"swap":        map[string]any{"current": 0, "total": 0},
		"netIO":       map[string]any{"down": 0, "up": 0},
		"netTraffic":  map[string]any{"recv": 0, "sent": 0},
		"publicIP":    map[string]any{"ipv4": "", "ipv6": ""},
		"tcpCount":    0,
		"udpCount":    0,
		"uptime":      0,
	}

	if first != 0 {
		if body, err := a.fleet.Ask(first, "status", nil); err == nil {
			var machine map[string]any
			if json.Unmarshal(body, &machine) == nil {
				for k, v := range machine {
					status[k] = v
				}
			}
		}
	}

	status["nodes"] = len(health)
	status["nodesOnline"] = online
	sendOK(w, status)
}

func (a *API) entrypointsList(w http.ResponseWriter, r *http.Request) {
	body, err := a.fleet.Read("entrypoints.list", nil)
	if err != nil {
		sendFail(w, err)
		return
	}

	var rows []struct {
		ID     int    `json:"id"`
		NodeID int    `json:"nodeId"`
		Port   int    `json:"port"`
		Remark string `json:"remark"`
		Enable bool   `json:"enable"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		sendFail(w, err)
		return
	}

	names := map[int]string{}
	exits := map[int]bool{}
	for _, n := range a.fleet.Nodes() {
		names[n.ID] = n.Tag
		exits[n.ID] = n.Role == "egress"
	}

	out := make([]map[string]any, 0, len(rows))
	for _, e := range rows {
		if exits[e.NodeID] {
			continue
		}
		label := names[e.NodeID]
		if label == "" {
			label = e.Remark
		}
		out = append(out, map[string]any{
			"id": e.ID, "tag": label, "port": e.Port, "nodeId": e.NodeID, "enable": e.Enable,
		})
	}
	sendOK(w, out)
}

var localPrefs = []string{"pageSize", "expireDiff"}

func (a *API) prefsMap() map[string]any {
	out := map[string]any{}
	if a.prefs == nil {
		return out
	}
	stored, err := a.prefs.Value(prefsKey)
	if err != nil || stored == "" {
		return out
	}
	var saved map[string]any
	if json.Unmarshal([]byte(stored), &saved) != nil {
		return out
	}
	for _, key := range localPrefs {
		if v, held := saved[key]; held {
			out[key] = v
		}
	}
	return out
}

func (a *API) settings(w http.ResponseWriter, r *http.Request) {
	current := map[string]any{"pageSize": 50, "expireDiff": 3}
	for k, v := range a.prefsMap() {
		current[k] = v
	}

	echSecret, echKnown := "", false
	if body, err := a.fleet.Read("network.settings", nil); err == nil {
		var network map[string]any
		if json.Unmarshal(body, &network) == nil {
			for _, key := range networkKeys {
				if v, carried := network[key]; carried {
					current[key] = v
				}
			}
			echSecret, _ = network["echSecret"].(string)
			echKnown = true
		}
	}
	needSecret := func() bool {
		name, _ := current["echName"].(string)
		return echKnown && name != "" && echSecret == ""
	}

	updating := r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/update")
	if !updating && needSecret() {
		a.write("network.settings.save", map[string]any{"echSecret": freshSecret()})
	}

	if updating {
		var patch map[string]any
		if err := bindBody(r, &patch); err != nil {
			sendFail(w, err)
			return
		}
		delete(patch, "clientReleases")
		if want, ok := patch["clientVersion"].(string); ok {
			if held, _ := current["clientVersion"].(string); want != held {
				releases, err := releasesFor(r.Context(), want)
				if err != nil {
					sendFail(w, err)
					return
				}
				patch["clientReleases"] = releases
			}
		}
		network := map[string]any{}
		for _, key := range networkKeys {
			v, carried := patch[key]
			if !carried {
				continue
			}
			network[key] = v
			current[key] = v
			delete(patch, key)
		}
		if needSecret() {
			network["echSecret"] = freshSecret()
		}
		if len(network) > 0 {
			written, err := a.write("network.settings.save", network)
			if err != nil {
				sendFail(w, err)
				return
			}
			for _, one := range written {
				if !one.Restarting {
					continue
				}
				current["restartMoved"] = one.Moved
				if took, why := a.fleet.Settled(network, settleWait); took {
					current["applied"] = true
				} else {
					current["applied"] = false
					current["applyError"] = why
				}
				break
			}
		}
		local := map[string]any{}
		for _, key := range localPrefs {
			if v, carried := patch[key]; carried {
				current[key] = v
			}
			local[key] = current[key]
		}
		if a.prefs != nil {
			blob, _ := json.Marshal(local)
			if err := a.prefs.SetValue(prefsKey, string(blob)); err != nil {
				sendFail(w, err)
				return
			}
		}
	}

	sendOK(w, current)
}

func (a *API) onlines(w http.ResponseWriter, r *http.Request) {
	rows, _, err := a.readClients()
	if err != nil {
		sendFail(w, err)
		return
	}

	tags := []string{}
	for _, row := range rows {
		if online(row) {
			tags = append(tags, textOf(row["tag"]))
		}
	}
	sort.Strings(tags)
	sendOK(w, tags)
}

func (a *API) lastOnline(w http.ResponseWriter, r *http.Request) {
	rows, _, err := a.readClients()
	if err != nil {
		sendFail(w, err)
		return
	}

	live := a.sessions()
	out := map[string]int64{}
	for _, row := range rows {
		if seen := live[qdcrypt.SessionID(textOf(row["uuid"]))]; seen.LastSeen > 0 {
			out[textOf(row["tag"])] = seen.LastSeen
		}
	}
	sendOK(w, out)
}

func (a *API) Live() map[string]any {
	out := map[string]any{"nodes": a.nodeRowsWithCounts()}

	clients, _, err := a.readClients()
	if err != nil {
		return out
	}

	onlineTags := []string{}
	stats := make([]map[string]any, 0, len(clients))
	for _, row := range clients {
		tag := textOf(row["tag"])
		if online(row) {
			onlineTags = append(onlineTags, tag)
		}

		stat := map[string]any{"tag": tag, "enable": row["enable"], "expiryAt": row["expiryAt"]}
		if traffic, ok := row["traffic"].(map[string]any); ok {
			for field, v := range traffic {
				stat[field] = v
			}
		}
		stats = append(stats, stat)
	}
	sort.Strings(onlineTags)

	out["traffic"] = map[string]any{"onlineClients": onlineTags}
	out["client_stats"] = map[string]any{"clients": stats}
	return out
}

type entryView struct {
	rows []map[string]any
	err  error
}

func (a *API) entrypoints() ([]map[string]any, error) {
	held := a.cache.get("entrypoints", func() any {
		rows, err := a.buildEntrypoints()
		return entryView{rows: rows, err: err}
	}).(entryView)
	return held.rows, held.err
}

func (a *API) buildEntrypoints() ([]map[string]any, error) {
	body, err := a.fleet.Read("entrypoints.list", nil)
	if err != nil {
		return nil, err
	}

	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

const themeKey = "panel.theme"

func (a *API) themeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/theme.json", a.themeRead)
	mux.HandleFunc("/panel/setting/theme", a.themeWrite)
}

func (a *API) themeRead(w http.ResponseWriter, r *http.Request) {
	blob, err := a.prefs.Value(themeKey)
	if err != nil || strings.TrimSpace(blob) == "" {
		blob = "{}"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(blob))
}

func (a *API) themeWrite(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		sendFail(w, err)
		return
	}

	var theme map[string]any
	if err := json.Unmarshal(body, &theme); err != nil {
		sendFail(w, fmt.Errorf("theme: %w", err))
		return
	}
	if err := a.prefs.SetValue(themeKey, string(body)); err != nil {
		sendFail(w, err)
		return
	}
	sendOK(w, theme)
}

const settleWait = 15 * time.Second
