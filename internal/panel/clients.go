package panel

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/netstate"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
)

type groupRow struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	EntrypointIDs []int  `json:"entrypointIds"`
	DeviceLimit   int    `json:"deviceLimit"`
	AllowExit     bool   `json:"allowExit"`
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

func (a *API) groupNameByID() (map[int]string, error) {
	rows, err := a.groups()
	if err != nil {
		return nil, err
	}
	out := make(map[int]string, len(rows))
	for _, g := range rows {
		out[g.ID] = g.Name
	}
	return out, nil
}

func (a *API) groupIDByName() (map[string]int, error) {
	rows, err := a.groups()
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, g := range rows {
		out[g.Name] = g.ID
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
		names[g.ID] = g.Name
		reach[g.ID] = g.EntrypointIDs
		tags = append(tags, g.Name)
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
		row["subId"] = uuid

		entries := reach[group]
		if entries == nil {
			entries = []int{}
		}
		row["inboundIds"] = entries

		row["uri"] = ""
		if uuid != "" {
			reachable := []clientstate.Endpoint{}
			for _, entry := range entries {
				address, known := where[entry]
				if !known {
					continue
				}
				host, port, err := net.SplitHostPort(address)
				if err != nil {
					continue
				}
				n, err := strconv.Atoi(port)
				if err != nil {
					continue
				}
				reachable = append(reachable, clientstate.Endpoint{Address: host, Port: n})
			}
			if len(reachable) > 0 {
				tag, _ := row["email"].(string)
				row["uri"] = clientstate.Link{
					Key: uuid, Label: tag, NetworkKey: key, Endpoints: reachable,
				}.String()
			}
		}

		seen := live[qdcrypt.SessionID(uuid)]
		saved := kept[int(numberOf(row["id"]))]

		up := saved.Traffic.Up + seen.Up
		down := saved.Traffic.Down + seen.Down

		lastOnline := seen.LastSeen
		if lastOnline == 0 {
			for _, a := range saved.Addresses {
				if a.LastSeen > lastOnline {
					lastOnline = a.LastSeen
				}
			}
		}

		row["traffic"] = map[string]any{
			"up": up, "down": down, "total": up + down,
			"enable": row["enable"], "expiryTime": row["expiryTime"],
			"lastOnline": lastOnline,
		}

		// Kept beside the network-wide figure so an entrypoint can show what
		// crossed its own node rather than the whole fleet's total.
		perNode := map[string]map[string]uint64{}
		for id, t := range saved.ByNode {
			perNode[strconv.Itoa(id)] = map[string]uint64{"up": t.Up, "down": t.Down}
		}
		if seen.NodeID != 0 {
			key := strconv.Itoa(seen.NodeID)
			held := perNode[key]
			if held == nil {
				held = map[string]uint64{}
			}
			held["up"] += seen.Up
			held["down"] += seen.Down
			perNode[key] = held
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
		row["ipLog"] = addresses
		row["exitLog"] = saved.Exits

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
	Session  uint32    `json:"session"`
	Client   string    `json:"client"`
	Transit  bool      `json:"transit"`
	LastSeen int64     `json:"lastSeen"`
	Since    int64     `json:"since"`
	Checked  int64     `json:"checked"`
	Device   string    `json:"device"`
	Up       uint64    `json:"up"`
	Down     uint64    `json:"down"`
	Seen     []address `json:"seen"`
	NodeID   int       `json:"-"`
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
	// The same figure split by the node that carried it: a client's row wants
	// the whole network, an entrypoint's row wants only its own node.
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

func (a *API) carried() map[int]carriedTotals {
	return a.cache.get("carried", a.askCarried).(map[int]carriedTotals)
}

func (a *API) askCarried() any {
	out := map[int]carriedTotals{}

	for _, n := range a.fleet.Live() {
		body, err := a.fleet.Ask(n.ID, "clients.stats", nil)
		if err != nil {
			continue
		}
		var answer struct {
			Carried map[string]carriedTotals `json:"carried"`
		}
		if json.Unmarshal(body, &answer) != nil {
			continue
		}
		for id, t := range answer.Carried {
			k, err := strconv.Atoi(id)
			if err != nil {
				continue
			}
			row := out[k]
			row.Up += t.Up
			row.Down += t.Down
			if t.At > row.At {
				row.At = t.At
			}
			out[k] = row
		}
	}
	return out
}

// Traffic is counted per node, so a figure for the whole network is the sum of
// what each node carried — not whatever one node's replica happens to hold.
// Nothing here is written back: the totals live only in this answer, so handing
// the database to a node never overwrites a client's history.
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
	for _, n := range a.fleet.Live() {
		nodeOf[n.ID] = true
		body, err := a.fleet.Ask(n.ID, "clients.stats", nil)
		if err != nil {
			continue
		}
		var answer stats
		if json.Unmarshal(body, &answer) != nil {
			continue
		}
		answers[n.ID] = answer
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
		// A node that predates the split reports no share of its own; falling
		// back to its whole replica beats showing nothing.
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

	// Only one node can be asked without the split, and then its replica already
	// holds every node's rows — adding them again would double the bytes.
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

	for _, node := range a.fleet.Live() {
		body, err := a.fleet.Ask(node.ID, "sessions", nil)
		if err != nil {
			continue
		}
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
				total.NodeID = node.ID
			}
			if row.Checked > total.Checked {
				total.Checked = row.Checked
			}
			if row.Device != "" {
				total.Device = row.Device
			}
			for _, ip := range row.Seen {
				ip.NodeID = node.ID
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

func (a *API) entrypointAddresses() map[int]string {
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

	out := make(map[int]string, len(rows))
	for _, e := range rows {
		if !e.Enable {
			continue
		}
		if host, known := hosts[e.NodeID]; known {
			out[e.ID] = fmt.Sprintf("%s:%d", host, e.Port)
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
			"id": g.ID, "name": g.Name, "clientCount": counts[g.ID],
			"entrypointIds": g.EntrypointIDs, "deviceLimit": g.DeviceLimit,
			"allowExit": g.AllowExit,
		})
	}
	sendOK(w, out)
}

func (a *API) clientsList(w http.ResponseWriter, r *http.Request) {
	rows, groups, err := a.readClients()
	if err != nil {
		sendFail(w, err)
		return
	}
	sendOK(w, rows)
	_ = groups
}

func (a *API) clientsPaged(w http.ResponseWriter, r *http.Request) {
	rows, groups, err := a.readClients()
	if err != nil {
		sendFail(w, err)
		return
	}

	// The panel has always sent ?search=; nothing here ever read it, so typing a
	// name filtered nothing. Matching is case-insensitive over the fields a
	// person would type: the tag and the comment.
	if needle := strings.TrimSpace(r.URL.Query().Get("search")); needle != "" {
		needle = strings.ToLower(needle)
		kept := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			tag := strings.ToLower(textOf(row["email"]))
			note := strings.ToLower(textOf(row["comment"]))
			if strings.Contains(tag, needle) || strings.Contains(note, needle) {
				kept = append(kept, row)
			}
		}
		rows = kept
	}

	page := atoiOr(r.URL.Query().Get("page"), 1)
	size := atoiOr(r.URL.Query().Get("pageSize"), 50)
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}

	from := (page - 1) * size
	if from > len(rows) {
		from = len(rows)
	}
	to := from + size
	if to > len(rows) {
		to = len(rows)
	}

	online := []string{}
	depleted := []string{}
	expiring := []string{}
	deactive := []string{}
	active := 0

	now := time.Now().UnixMilli()
	soon := now + a.expiryWarning()

	for _, row := range rows {
		email := textOf(row["email"])
		enabled, _ := row["enable"].(bool)
		expiry := int64(numberOf(row["expiryTime"]))

		if !enabled {
			deactive = append(deactive, email)
			continue
		}

		if numberOf(row["onlineSince"]) > 0 {
			online = append(online, email)
		}

		// Active means still carrying traffic: enabled and not run out. A client
		// about to expire is warned about, not counted as gone.
		if expiry > 0 && expiry < now {
			depleted = append(depleted, email)
			continue
		}
		active++
		if expiry > 0 && expiry < soon {
			expiring = append(expiring, email)
		}
	}

	sendOK(w, map[string]any{
		"items":    rows[from:to],
		"total":    len(rows),
		"filtered": len(rows),
		"page":     page,
		"pageSize": size,
		"groups":   groups,
		"summary": map[string]any{
			"total":    len(rows),
			"active":   active,
			"online":   online,
			"depleted": depleted,
			"expiring": expiring,
			"deactive": deactive,
		},
	})
}

func (a *API) expiryWarning() int64 {
	days := 3.0
	if a.prefs != nil {
		if stored, err := a.prefs.Value(prefsKey); err == nil && stored != "" {
			var saved map[string]any
			if json.Unmarshal([]byte(stored), &saved) == nil {
				if set := numberOf(saved["expireDiff"]); set > 0 {
					days = set
				}
			}
		}
	}
	return int64(days * 24 * 60 * 60 * 1000)
}

func atoiOr(text string, fallback int) int {
	if n, err := strconv.Atoi(text); err == nil {
		return n
	}
	return fallback
}

func (a *API) clientSave(id int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.writeClient(w, r, id)
	}
}

func (a *API) clientSaveWithID(w http.ResponseWriter, r *http.Request) {
	segment := pathTail(r.URL.Path, "/panel/api/clients/update/")

	if id, err := strconv.Atoi(segment); err == nil {
		a.writeClient(w, r, id)
		return
	}

	byEmail, err := a.clientIDs()
	if err != nil {
		sendFail(w, err)
		return
	}
	id, known := byEmail[segment]
	if !known {
		sendFail(w, fmt.Errorf("no client named %q", segment))
		return
	}
	a.writeClient(w, r, id)
}

func (a *API) clientDelete(w http.ResponseWriter, r *http.Request) {
	segment := pathTail(r.URL.Path, "/panel/api/clients/del/")

	id, err := strconv.Atoi(segment)
	if err != nil {
		byEmail, lookupErr := a.clientIDs()
		if lookupErr != nil {
			sendFail(w, lookupErr)
			return
		}
		known := false
		if id, known = byEmail[segment]; !known {
			sendFail(w, fmt.Errorf("no client named %q", segment))
			return
		}
	}

	results, err := a.write("clients.delete", map[string]int{"id": id})
	if err != nil {
		sendFailWith(w, err, results)
		return
	}
	sendOK(w, map[string]any{"nodes": results})
}

func (a *API) writeClient(w http.ResponseWriter, r *http.Request, id int) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		sendFail(w, err)
		return
	}

	var body map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			sendFail(w, err)
			return
		}
	}
	if body == nil {
		body = map[string]any{}
	}
	if nested, ok := body["client"].(map[string]any); ok {
		body = nested
	}

	uuid := firstText(body["subId"], body["uuid"], body["id"])
	delete(body, "subId")
	delete(body, "id")
	if uuid != "" {
		body["uuid"] = uuid
	} else {
		delete(body, "uuid")
	}

	if id != 0 {
		body["id"] = id
	}

	if name, ok := body["group"].(string); ok {
		delete(body, "group")
		body["groupId"] = 0
		if name != "" {
			ids, err := a.groupIDByName()
			if err != nil {
				sendFail(w, err)
				return
			}
			groupID, known := ids[name]
			if !known {
				sendFail(w, errUnknownGroup(name))
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

func firstText(values ...any) string {
	for _, v := range values {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

type errUnknownGroup string

func (e errUnknownGroup) Error() string { return "no group named " + string(e) }
