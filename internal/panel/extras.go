package panel

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jaywehosl/quic-diver/internal/netstate"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
)

const prefsKey = "panel.prefs"

func (a *API) extraRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/panel/api/server/status", a.serverStatus)
	mux.HandleFunc("/panel/api/inbounds/options", a.inboundOptions)

	mux.HandleFunc("/panel/api/clients/onlines", a.onlines)
	mux.HandleFunc("/panel/api/clients/onlinesByNode", a.onlinesByNode)
	mux.HandleFunc("/panel/api/clients/lastOnline", a.lastOnline)
	mux.HandleFunc("/panel/api/clients/activeInbounds", emptyObject)

	mux.HandleFunc("/panel/api/publish/draft", a.draft)
	mux.HandleFunc("/panel/setting/all", a.settings)
	mux.HandleFunc("/panel/setting/defaultSettings", a.settings)
	mux.HandleFunc("/panel/setting/update", a.settings)
	mux.HandleFunc("/panel/setting/restartPanel", a.restartAll)

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

func (a *API) inboundOptions(w http.ResponseWriter, r *http.Request) {
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
		label := e.Remark
		if label == "" {
			label = names[e.NodeID]
		}
		out = append(out, map[string]any{
			"id": e.ID, "remark": label, "port": e.Port, "nodeId": e.NodeID,
			"protocol": "qd", "enable": true,
		})
	}
	sendOK(w, out)
}

func (a *API) draft(w http.ResponseWriter, r *http.Request) {
	revision := 0
	for _, h := range a.fleet.Health() {
		if h.Revision > revision {
			revision = h.Revision
		}
	}
	sendOK(w, map[string]any{
		"changes":           []any{},
		"revision":          revision,
		"publishedRevision": revision,
	})
}

func (a *API) settings(w http.ResponseWriter, r *http.Request) {
	current := map[string]any{
		"pageSize":     50,
		"timeLocation": time.Local.String(),
		"datepicker":   "gregorian",
		// The same window the summary warns by, in days. Reporting 0 here left
		// the panel recomputing the counts with no warning window at all, so a
		// client about to expire never showed up as depleting.
		"expireDiff":      float64(a.expiryWarning()) / (24 * 60 * 60 * 1000),
		"trafficDiff":     0,
		"remarkModel":     "-ieo",
		"subEnable":       false,
		"tgBotEnable":     false,
		"twoFactorEnable": false,
	}

	if body, err := a.fleet.Read("network.settings", nil); err == nil {
		var network map[string]any
		if json.Unmarshal(body, &network) == nil {
			for _, key := range networkKeys {
				if v, carried := network[key]; carried {
					current[key] = v
				}
			}
		}
	}

	if a.prefs != nil {
		if stored, err := a.prefs.Value(prefsKey); err == nil && stored != "" {
			var saved map[string]any
			if json.Unmarshal([]byte(stored), &saved) == nil {
				for _, key := range networkKeys {
					delete(saved, key)
				}
				for k, v := range saved {
					current[k] = v
				}
			}
		}
	}

	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/update") {
		var patch map[string]any
		if err := bindBody(r, &patch); err != nil {
			sendFail(w, err)
			return
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
		if len(network) > 0 {
			written, err := a.write("network.settings.save", network)
			if err != nil {
				sendFail(w, err)
				return
			}
			// Правку узел мог принять, но применить лишь после перезапуска. Тогда
			// дожидаемся, пока он поднимется с ней, и отвечаем по факту: вступило
			// или нет. Иначе «сохранено» значило бы всего лишь «записано в базу».
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
		for k, v := range patch {
			current[k] = v
		}
		if a.prefs != nil {
			local := map[string]any{}
			for k, v := range current {
				local[k] = v
			}
			for _, key := range networkKeys {
				delete(local, key)
			}
			blob, _ := json.Marshal(local)
			if err := a.prefs.SetValue(prefsKey, string(blob)); err != nil {
				sendFail(w, err)
				return
			}
		}
	}

	sendOK(w, current)
}

func present(s sessionStat) bool { return s.Since > 0 }

func (a *API) presence() (map[string]sessionStat, error) {
	rows, _, err := a.readClients()
	if err != nil {
		return nil, err
	}

	live := a.sessions()
	out := make(map[string]sessionStat, len(rows))
	for _, row := range rows {
		uuid, _ := row["uuid"].(string)
		tag, _ := row["email"].(string)
		if uuid == "" || tag == "" {
			continue
		}
		if seen, carried := live[qdcrypt.SessionID(uuid)]; carried {
			out[tag] = seen
		}
	}
	return out, nil
}

func (a *API) onlines(w http.ResponseWriter, r *http.Request) {
	seen, err := a.presence()
	if err != nil {
		sendFail(w, err)
		return
	}

	names := []string{}
	for tag, stat := range seen {
		if present(stat) {
			names = append(names, tag)
		}
	}
	sort.Strings(names)
	sendOK(w, names)
}

func (a *API) onlinesByNode(w http.ResponseWriter, r *http.Request) {
	seen, err := a.presence()
	if err != nil {
		sendFail(w, err)
		return
	}

	byNode := map[string][]string{}
	for tag, stat := range seen {
		if !present(stat) {
			continue
		}
		node := strconv.Itoa(stat.NodeID)
		byNode[node] = append(byNode[node], tag)
	}
	for _, names := range byNode {
		sort.Strings(names)
	}
	sendOK(w, byNode)
}

func (a *API) lastOnline(w http.ResponseWriter, r *http.Request) {
	seen, err := a.presence()
	if err != nil {
		sendFail(w, err)
		return
	}

	out := map[string]int64{}
	for tag, stat := range seen {
		if stat.LastSeen > 0 {
			out[tag] = stat.LastSeen
		}
	}
	sendOK(w, out)
}

func emptyObject(w http.ResponseWriter, r *http.Request) {
	sendOK(w, map[string]any{})
}

func emptyList(w http.ResponseWriter, r *http.Request) {
	sendOK(w, []any{})
}

func (a *API) entrypointsList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.entrypoints()
	if err != nil {
		sendFail(w, err)
		return
	}
	sendOK(w, rows)
}

func (a *API) Live() map[string]any {
	out := map[string]any{"nodes": a.nodeRowsWithCounts()}
	if rows, err := a.entrypoints(); err == nil {
		out["inbounds"] = rows
	}

	clients, _, err := a.readClients()
	if err != nil {
		return out
	}

	online := []string{}
	stats := make([]map[string]any, 0, len(clients))
	for _, row := range clients {
		tag := textOf(row["email"])
		if tag == "" {
			continue
		}
		if numberOf(row["onlineSince"]) > 0 {
			online = append(online, tag)
		}

		stat := map[string]any{"email": tag}
		if traffic, ok := row["traffic"].(map[string]any); ok {
			for _, field := range []string{"up", "down", "total", "enable", "expiryTime", "lastOnline"} {
				stat[field] = traffic[field]
			}
		}
		stats = append(stats, stat)
	}
	sort.Strings(online)

	out["traffic"] = map[string]any{"onlineClients": online}
	out["client_stats"] = map[string]any{"clients": stats}
	return out
}

type entryView struct {
	rows []map[string]any
	err  error
}

// An entrypoint lives on one node, so its traffic is what that node carried for
// the clients reachable through it — not the fleet-wide figure the client rows
// show. Pass node 0 to fall back to the whole network.
func (a *API) throughEntry(entry int, node int) ([]map[string]any, float64, float64) {
	clients, _, err := a.readClients()
	if err != nil {
		return []map[string]any{}, 0, 0
	}

	stats := []map[string]any{}
	var up, down float64

	for _, c := range clients {
		ids, ok := c["inboundIds"].([]int)
		if !ok {
			continue
		}
		reaches := false
		for _, id := range ids {
			if id == entry {
				reaches = true
				break
			}
		}
		if !reaches {
			continue
		}

		row := map[string]any{
			"email":      textOf(c["email"]),
			"enable":     c["enable"],
			"expiryTime": c["expiryTime"],
		}
		mine, split := c["trafficByNode"].(map[string]map[string]uint64)
		if node != 0 && split {
			held := mine[strconv.Itoa(node)]
			rowUp, rowDown := float64(held["up"]), float64(held["down"])
			row["up"] = rowUp
			row["down"] = rowDown
			row["total"] = rowUp + rowDown
			up += rowUp
			down += rowDown
		} else if traffic, ok := c["traffic"].(map[string]any); ok {
			row["up"] = traffic["up"]
			row["down"] = traffic["down"]
			row["total"] = traffic["total"]
			up += numberOf(traffic["up"])
			down += numberOf(traffic["down"])
		}
		stats = append(stats, row)
	}
	return stats, up, down
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

	names := map[int]string{}
	roles := map[int]string{}
	for _, n := range a.fleet.Nodes() {
		names[n.ID] = n.Tag
		roles[n.ID] = n.Role
	}

	for _, row := range rows {
		nodeID, _ := row["nodeId"].(float64)
		port, _ := row["port"].(float64)

		row["protocol"] = "qd"
		row["listen"] = "0.0.0.0"
		row["tag"] = fmt.Sprintf("%s:%d", names[int(nodeID)], int(port))
		row["settings"] = `{"clients":[]}`
		row["streamSettings"] = `{}`
		row["sniffing"] = `{}`
		id := int(numberOf(row["id"]))
		stats, up, down := a.throughEntry(id, int(nodeID))

		count := len(stats)
		if roles[int(nodeID)] == string(netstate.RoleEgress) {
			if t, carried := a.carried()[int(nodeID)]; carried {
				up, down = float64(t.Up), float64(t.Down)
			}
			if n := a.carrying()[int(nodeID)]; n > 0 {
				count = n
			}
		}

		row["clientStats"] = stats
		row["up"] = up
		row["down"] = down
		row["total"] = up + down
		row["clientCount"] = count
		row["expiryTime"] = 0
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

// settleWait — сколько ждать возвращения узла с новой настройкой. Перезапуск
// занимает около секунды, остальное — запас на медленную машину.
const settleWait = 15 * time.Second

func (a *API) carrying() map[int]int {
	out := map[int]int{}
	for _, h := range a.fleet.Seen() {
		out[h.ID] = h.Carrying
	}
	return out
}
