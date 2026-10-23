package panel

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/netstate"
	"github.com/jaywehosl/qd/internal/qdcrypt"
)

type groupRow struct {
	ID            int    `json:"id"`
	Tag           string `json:"tag"`
	EntrypointIDs []int  `json:"entrypointIds"`
	DeviceLimit   int    `json:"deviceLimit"`
	AllowExit     bool   `json:"allowExit"`
	AllowDev      bool   `json:"allowDev"`
	AllowCore     bool   `json:"allowCore"`
}

func (a *API) groups() ([]groupRow, error) {
	held := a.cache.get("groups", func() any {
		body, err := a.fleet.Read("groups.list", nil)
		if err != nil {
			return groupView{err: err}
		}
		var rows []groupRow
		if err := json.Unmarshal(body, &rows); err != nil {
			return groupView{err: err}
		}
		return groupView{rows: rows}
	}).(groupView)
	return held.rows, held.err
}

type groupView struct {
	rows []groupRow
	err  error
}

func (a *API) groupIDByTag() (map[string]int, error) {
	rows, err := a.groups()
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, g := range rows {
		out[g.Tag] = g.ID
	}
	return out, nil
}

type clientView struct {
	rows   []map[string]any
	groups []string
	err    error
}

func (a *API) readClients() ([]map[string]any, []string, error) {
	view := a.cache.get("clients", func() any {
		rows, groups, err := a.buildClients()
		return clientView{rows: rows, groups: groups, err: err}
	}).(clientView)
	return view.rows, view.groups, view.err
}

func (a *API) buildClients() ([]map[string]any, []string, error) {
	body, err := a.fleet.Read("clients.list", nil)
	if err != nil {
		return nil, nil, err
	}

	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, nil, err
	}

	groups, err := a.groups()
	if err != nil {
		return nil, nil, err
	}
	names := make(map[int]string, len(groups))
	reach := make(map[int][]int, len(groups))
	tags := make([]string, 0, len(groups))
	for _, g := range groups {
		names[g.ID] = g.Tag
		reach[g.ID] = g.EntrypointIDs
		tags = append(tags, g.Tag)
	}

	where := a.entrypointAddresses()
	key := hex.EncodeToString(a.fleet.key[:])
	live := a.sessions()
	kept := a.stored()

	for _, row := range rows {
		id, _ := row["groupId"].(float64)
		group := int(id)

		row["group"] = names[group]
		delete(row, "groupId")

		uuid, _ := row["uuid"].(string)

		entries := reach[group]
		if entries == nil {
			entries = []int{}
		}
		row["entrypointIds"] = entries

		row["uri"] = ""
		if uuid != "" {
			reachable := []clientstate.Endpoint{}
			nodeEndpoint := map[int]string{}
			for _, entry := range entries {
				at, known := where[entry]
				if !known {
					continue
				}
				reachable = append(reachable, at.Endpoint)
				if _, held := nodeEndpoint[at.node]; !held {
					nodeEndpoint[at.node] = at.Endpoint.String()
				}
			}

			if len(reachable) > 0 {
				tag, _ := row["tag"].(string)
				row["uri"] = clientstate.Link{
					Key: uuid, Label: tag, NetworkKey: key, Endpoints: reachable,
				}.String()
			}
		}

		seen := live[qdcrypt.SessionID(uuid)]
		saved := kept[int(numberOf(row["id"]))]

		up := saved.Traffic.Up
		down := saved.Traffic.Down

		lastOnline := seen.LastSeen
		if lastOnline == 0 {
			for _, a := range saved.Addresses {
				if a.LastSeen > lastOnline {
					lastOnline = a.LastSeen
				}
			}
		}

		row["traffic"] = map[string]any{"up": up, "down": down, "lastOnline": lastOnline}

		perNode := map[string]map[string]uint64{}
		for id, t := range saved.ByNode {
			perNode[strconv.Itoa(id)] = map[string]uint64{"up": t.Up, "down": t.Down}
		}
		row["trafficByNode"] = perNode
		row["lastNodeId"] = seen.NodeID

		if lastOnline > 0 {
			row["lastConnect"] = lastOnline
		}
		checked := lastOnline
		if seen.Checked > checked {
			checked = seen.Checked
		}
		for _, d := range saved.Devices {
			if d.LastSeen > checked {
				checked = d.LastSeen
			}
		}
		if checked > 0 {
			row["updatedAt"] = checked
		}
		if seen.Since > 0 {
			row["onlineSince"] = seen.Since
		}

		fresh := seen.Seen
		if seen.Transit {
			fresh = nil
		}
		addresses := merge(saved.Addresses, fresh)
		row["addresses"] = addresses
		row["exits"] = saved.Exits

		devices := make([]deviceRow, 0, len(saved.Devices))
		for _, d := range saved.Devices {
			for _, a := range addresses {
				if a.Fingerprint == d.Fingerprint {
					d.IP = a.IP
					break
				}
			}
			devices = append(devices, d)
		}
		row["devices"] = devices
		if len(addresses) > 0 && numberOf(row["lastConnect"]) == 0 {
			row["lastConnect"] = addresses[0].LastSeen
		}
	}
	return rows, tags, nil
}

type sessionStat struct {
	Session  uint32       `json:"session"`
	Client   string       `json:"client"`
	Transit  bool         `json:"transit"`
	LastSeen int64        `json:"lastSeen"`
	Since    int64        `json:"since"`
	Checked  int64        `json:"checked"`
	Device   string       `json:"device"`
	Up       uint64       `json:"up"`
	Down     uint64       `json:"down"`
	Seen     []address    `json:"seen"`
	NodeID   int          `json:"-"`
	On       map[int]bool `json:"-"`
}

type address struct {
	IP          string `json:"ip"`
	Fingerprint string `json:"fingerprint"`
	FirstSeen   int64  `json:"firstSeen"`
	LastSeen    int64  `json:"lastOnline"`
	NodeID      int    `json:"nodeId"`
}

type keptStats struct {
	Traffic struct {
		Up   uint64 `json:"up"`
		Down uint64 `json:"down"`
		At   int64  `json:"at"`
	}
	ByNode    map[int]carriedTotals
	Addresses []address
	Devices   []deviceRow
	Exits     []exitRow
}

type exitRow struct {
	NodeID    int    `json:"nodeId"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastOnline"`
	Up        uint64 `json:"up"`
	Down      uint64 `json:"down"`
}

func mergeExits(held, fresh []exitRow) []exitRow {
	by := map[int]exitRow{}
	for _, e := range append(append([]exitRow{}, held...), fresh...) {
		was, seen := by[e.NodeID]
		if !seen {
			by[e.NodeID] = e
			continue
		}
		if e.LastSeen > was.LastSeen {
			was.LastSeen = e.LastSeen
		}
		if was.FirstSeen == 0 || (e.FirstSeen > 0 && e.FirstSeen < was.FirstSeen) {
			was.FirstSeen = e.FirstSeen
		}
		if e.Up > was.Up {
			was.Up = e.Up
		}
		if e.Down > was.Down {
			was.Down = e.Down
		}
		by[e.NodeID] = was
	}

	out := make([]exitRow, 0, len(by))
	for _, e := range by {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen > out[j].LastSeen })
	return out
}

type deviceRow struct {
	Fingerprint string `json:"fingerprint"`
	Platform    string `json:"platform"`
	Model       string `json:"model"`
	Kind        string `json:"kind"`
	Version     string `json:"version"`
	Blocked     bool   `json:"blocked"`
	IP          string `json:"ip"`
	NodeID      int    `json:"nodeId"`
	FirstSeen   int64  `json:"firstSeen"`
	LastSeen    int64  `json:"lastSeen"`
	Up          uint64 `json:"up"`
	Down        uint64 `json:"down"`
}

func (a *API) stored() map[int]keptStats {
	return a.cache.get("stats", a.askStored).(map[int]keptStats)
}

type carriedTotals struct {
	Up   uint64 `json:"up"`
	Down uint64 `json:"down"`
	At   int64  `json:"at"`
}

func (a *API) askStored() any {
	out := map[int]keptStats{}

	type totals struct {
		Up   uint64 `json:"up"`
		Down uint64 `json:"down"`
		At   int64  `json:"at"`
	}
	type stats struct {
		Traffic   map[string]totals      `json:"traffic"`
		Mine      map[string]totals      `json:"mine"`
		Addresses map[string][]address   `json:"addresses"`
		Devices   map[string][]deviceRow `json:"devices"`
		Exits     map[string][]exitRow   `json:"exits"`
	}

	answers := map[int]stats{}
	nodeOf := map[int]bool{}
	for id, body := range a.fleet.Gather("clients.stats", nil) {
		nodeOf[id] = true
		var answer stats
		if json.Unmarshal(body, &answer) != nil {
			continue
		}
		answers[id] = answer
	}
	if len(answers) == 0 {
		return out
	}

	shares := false
	for _, answer := range answers {
		if len(answer.Mine) > 0 {
			shares = true
			break
		}
	}

	for nodeID, answer := range answers {
		counted := answer.Mine
		if !shares {
			counted = answer.Traffic
		} else if len(counted) == 0 {
			counted = nil
		}

		for id, t := range counted {
			n, err := strconv.Atoi(id)
			if err != nil {
				continue
			}
			row := out[n]
			row.Traffic.Up += t.Up
			row.Traffic.Down += t.Down
			if t.At > row.Traffic.At {
				row.Traffic.At = t.At
			}
			if row.ByNode == nil {
				row.ByNode = map[int]carriedTotals{}
			}
			held := row.ByNode[nodeID]
			held.Up += t.Up
			held.Down += t.Down
			if t.At > held.At {
				held.At = t.At
			}
			row.ByNode[nodeID] = held
			out[n] = row
		}

		for id, seen := range answer.Addresses {
			n, err := strconv.Atoi(id)
			if err != nil {
				continue
			}
			row := out[n]
			row.Addresses = merge(row.Addresses, seen)
			out[n] = row
		}

		for id, seen := range answer.Devices {
			n, err := strconv.Atoi(id)
			if err != nil {
				continue
			}
			row := out[n]
			row.Devices = mergeDevices(row.Devices, seen)
			out[n] = row
		}

		for id, seen := range answer.Exits {
			n, err := strconv.Atoi(id)
			if err != nil {
				continue
			}
			row := out[n]
			row.Exits = mergeExits(row.Exits, seen)
			out[n] = row
		}
	}

	if !shares && len(answers) > 1 {
		best := map[int]totals{}
		for _, answer := range answers {
			for id, t := range answer.Traffic {
				n, err := strconv.Atoi(id)
				if err != nil {
					continue
				}
				if held, seen := best[n]; !seen || t.Up+t.Down > held.Up+held.Down {
					best[n] = t
				}
			}
		}
		for n, t := range best {
			row := out[n]
			row.Traffic.Up, row.Traffic.Down, row.Traffic.At = t.Up, t.Down, t.At
			out[n] = row
		}
	}

	return out
}

func mergeDevices(kept, live []deviceRow) []deviceRow {
	byPrint := map[string]deviceRow{}
	for _, d := range append(append([]deviceRow{}, kept...), live...) {
		if held, seen := byPrint[d.Fingerprint]; seen && held.LastSeen >= d.LastSeen {
			continue
		}
		byPrint[d.Fingerprint] = d
	}

	out := make([]deviceRow, 0, len(byPrint))
	for _, d := range byPrint {
		out = append(out, d)
	}
	return out
}

func merge(kept, live []address) []address {
	byIP := map[string]address{}
	for _, a := range append(append([]address{}, kept...), live...) {
		if held, seen := byIP[a.IP]; seen && held.LastSeen >= a.LastSeen {
			continue
		}
		byIP[a.IP] = a
	}

	out := make([]address, 0, len(byIP))
	for _, a := range byIP {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen > out[j].LastSeen })
	return out
}

func (a *API) sessions() map[uint32]sessionStat {
	return a.cache.get("sessions", a.askSessions).(map[uint32]sessionStat)
}

func (a *API) askSessions() any {
	out := map[uint32]sessionStat{}

	for id, body := range a.fleet.Gather("sessions", nil) {
		var rows []sessionStat
		if json.Unmarshal(body, &rows) != nil {
			continue
		}
		for _, row := range rows {
			total := out[row.Session]
			total.Session = row.Session
			total.Up += row.Up
			total.Down += row.Down
			if row.LastSeen > total.LastSeen {
				total.LastSeen = row.LastSeen
				total.Client = row.Client
				total.Since = row.Since
				total.NodeID = id
			}
			if row.Checked > total.Checked {
				total.Checked = row.Checked
			}
			if row.Since > 0 {
				if total.On == nil {
					total.On = map[int]bool{}
				}
				total.On[id] = true
			}
			if row.Device != "" {
				total.Device = row.Device
			}
			for _, ip := range row.Seen {
				ip.NodeID = id
				if ip.Fingerprint == "" {
					ip.Fingerprint = row.Device
				}
				total.Seen = append(total.Seen, ip)
			}
			out[row.Session] = total
		}
	}
	return out
}

type entryAt struct {
	clientstate.Endpoint
	node int
}

func (a *API) entrypointAddresses() map[int]entryAt {
	body, err := a.fleet.Read("entrypoints.list", nil)
	if err != nil {
		return nil
	}
	var rows []struct {
		ID     int  `json:"id"`
		NodeID int  `json:"nodeId"`
		Port   int  `json:"port"`
		Enable bool `json:"enable"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil
	}

	hosts := map[int]string{}
	for _, n := range a.fleet.Nodes() {
		if !n.Enable || n.Address == "" || n.Role == string(netstate.RoleEgress) {
			continue
		}
		hosts[n.ID] = n.Address
	}

	out := make(map[int]entryAt, len(rows))
	for _, e := range rows {
		if !e.Enable {
			continue
		}
		if host, known := hosts[e.NodeID]; known {
			out[e.ID] = entryAt{Endpoint: clientstate.Endpoint{Address: host, Port: e.Port}, node: e.NodeID}
		}
	}
	return out
}

func (a *API) groupsList(w http.ResponseWriter, r *http.Request) {
	groups, err := a.groups()
	if err != nil {
		sendFail(w, err)
		return
	}

	body, err := a.fleet.Read("clients.list", nil)
	if err != nil {
		sendFail(w, err)
		return
	}
	var clients []struct {
		GroupID int `json:"groupId"`
	}
	json.Unmarshal(body, &clients)

	counts := map[int]int{}
	for _, c := range clients {
		counts[c.GroupID]++
	}

	out := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		out = append(out, map[string]any{
			"id": g.ID, "tag": g.Tag, "clientCount": counts[g.ID],
			"entrypointIds": g.EntrypointIDs, "deviceLimit": g.DeviceLimit,
			"allowExit": g.AllowExit, "allowDev": g.AllowDev, "allowCore": g.AllowCore,
		})
	}
	sendOK(w, out)
}

func (a *API) clientsList(w http.ResponseWriter, r *http.Request) {
	rows, _, err := a.readClients()
	if err != nil {
		sendFail(w, err)
		return
	}
	sendOK(w, rows)
}

func standing(row map[string]any, now, soon int64) string {
	expiry := int64(numberOf(row["expiryAt"]))
	switch {
	case expiry > 0 && expiry <= now:
		return "expired"
	case row["enable"] != true:
		return "disabled"
	case expiry > 0 && expiry < soon:
		return "expiring"
	}
	return "active"
}

func online(row map[string]any) bool {
	return row["enable"] == true && numberOf(row["onlineSince"]) > 0
}

func (a *API) clientsPaged(w http.ResponseWriter, r *http.Request) {
	rows, groups, err := a.readClients()
	if err != nil {
		sendFail(w, err)
		return
	}
	query := r.URL.Query()
	now := time.Now().UnixMilli()
	soon := now + a.expiryWarning()

	summary := map[string]any{"total": len(rows)}
	lists := map[string][]string{"online": {}, "expired": {}, "expiring": {}, "disabled": {}}
	active := 0
	for _, row := range rows {
		tag := textOf(row["tag"])
		if online(row) {
			lists["online"] = append(lists["online"], tag)
		}
		switch state := standing(row, now, soon); state {
		case "active":
			active++
		case "expiring":
			active++
			lists[state] = append(lists[state], tag)
		default:
			lists[state] = append(lists[state], tag)
		}
	}
	summary["active"] = active
	for name, tags := range lists {
		summary[name] = tags
	}

	needle := strings.ToLower(strings.TrimSpace(query.Get("search")))
	group := query.Get("group")
	status := query.Get("status")
	kept := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if needle != "" && !strings.Contains(strings.ToLower(textOf(row["tag"])), needle) &&
			!strings.Contains(strings.ToLower(textOf(row["comment"])), needle) {
			continue
		}
		if group != "" && textOf(row["group"]) != group {
			continue
		}
		switch state := standing(row, now, soon); status {
		case "":
		case "online":
			if !online(row) {
				continue
			}
		case "active":
			if state != "active" && state != "expiring" {
				continue
			}
		default:
			if state != status {
				continue
			}
		}
		kept = append(kept, row)
	}
	sortClients(kept, query.Get("sort"), query.Get("order") == "descend")

	page := max(atoiOr(query.Get("page"), 1), 1)
	size := atoiOr(query.Get("pageSize"), 50)
	if size < 1 {
		size = 50
	}
	from := min((page-1)*size, len(kept))
	to := min(from+size, len(kept))

	sendOK(w, map[string]any{
		"items":    kept[from:to],
		"total":    len(rows),
		"filtered": len(kept),
		"page":     page,
		"pageSize": size,
		"groups":   groups,
		"summary":  summary,
	})
}

func sortClients(rows []map[string]any, by string, descend bool) {
	key := func(row map[string]any) float64 {
		switch by {
		case "updatedAt", "expiryAt":
			return numberOf(row[by])
		case "lastOnline":
			traffic, _ := row["traffic"].(map[string]any)
			return numberOf(traffic["lastOnline"])
		case "traffic":
			traffic, _ := row["traffic"].(map[string]any)
			return numberOf(traffic["up"]) + numberOf(traffic["down"])
		}
		return numberOf(row["createdAt"])
	}
	less := func(i, j int) bool { return key(rows[i]) < key(rows[j]) }
	if by == "tag" {
		less = func(i, j int) bool {
			return strings.ToLower(textOf(rows[i]["tag"])) < strings.ToLower(textOf(rows[j]["tag"]))
		}
	}
	if by == "expiryAt" {
		plain := less
		less = func(i, j int) bool {
			a, b := numberOf(rows[i]["expiryAt"]), numberOf(rows[j]["expiryAt"])
			if (a == 0) != (b == 0) {
				return b == 0
			}
			return plain(i, j)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if descend {
			return less(j, i)
		}
		return less(i, j)
	})
}

func (a *API) expiryWarning() int64 {
	days := numberOf(a.prefsMap()["expireDiff"])
	if days <= 0 {
		days = 3
	}
	return int64(days * 24 * 60 * 60 * 1000)
}

func atoiOr(text string, fallback int) int {
	if n, err := strconv.Atoi(text); err == nil {
		return n
	}
	return fallback
}

func (a *API) clientAdd(w http.ResponseWriter, r *http.Request) {
	a.writeClient(w, r, 0)
}

func (a *API) clientUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(lastPathSegment(r.URL.Path))
	if err != nil || id <= 0 {
		sendFail(w, fmt.Errorf("no client id in %q", r.URL.Path))
		return
	}
	a.writeClient(w, r, id)
}

func (a *API) clientDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(lastPathSegment(r.URL.Path))
	if err != nil || id <= 0 {
		sendFail(w, fmt.Errorf("no client id in %q", r.URL.Path))
		return
	}
	results, err := a.write("clients.delete", map[string]int{"id": id})
	if err != nil {
		sendFailWith(w, err, results)
		return
	}
	sendOK(w, map[string]any{"nodes": results})
}

func (a *API) writeClient(w http.ResponseWriter, r *http.Request, id int) {
	body := map[string]any{}
	if err := bindBody(r, &body); err != nil {
		sendFail(w, err)
		return
	}
	delete(body, "id")
	if id != 0 {
		body["id"] = id
	} else if textOf(body["uuid"]) == "" {
		body["uuid"] = newUUID()
	}

	if tag, ok := body["group"].(string); ok {
		delete(body, "group")
		body["groupId"] = 0
		if tag != "" {
			ids, err := a.groupIDByTag()
			if err != nil {
				sendFail(w, err)
				return
			}
			groupID, known := ids[tag]
			if !known {
				sendFail(w, fmt.Errorf("no group %q", tag))
				return
			}
			body["groupId"] = groupID
		}
	}

	results, err := a.write("clients.save", body)
	if err != nil {
		sendFailWith(w, err, results)
		return
	}
	sendOK(w, map[string]any{"nodes": results})
}
