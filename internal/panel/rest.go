package panel

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
)

func (a *API) restRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/panel/api/nodes/probe/", a.nodeProbe)
	mux.HandleFunc("/panel/api/nodes/setEnable/", a.nodeSetEnable)

	mux.HandleFunc("/panel/api/clients/get/", a.clientGet)
	mux.HandleFunc("/panel/api/clients/bulkDel", a.clientsBulkDelete)
	mux.HandleFunc("/panel/api/clients/bulkCreate", a.clientsBulkCreate)
	mux.HandleFunc("/panel/api/clients/bulkAdjust", a.clientsBulkAdjust)
	mux.HandleFunc("/panel/api/clients/delDepleted", a.clientsDelDepleted)
	mux.HandleFunc("/panel/api/clients/resetTraffic/", a.resetTraffic)
	mux.HandleFunc("/panel/api/clients/resetAllTraffics", a.resetTraffic)

	mux.HandleFunc("/panel/api/clients/groups/bulkAdd", a.groupBulkAdd)
	mux.HandleFunc("/panel/api/clients/groups/bulkRemove", a.groupBulkRemove)
	mux.HandleFunc("/panel/api/clients/groups/rename", a.groupRename)
	mux.HandleFunc("/panel/api/clients/groups/entrypoints", a.groupEntrypoints)

	mux.HandleFunc("/panel/api/inbounds/get/", a.entrypointGet)
	mux.HandleFunc("/panel/api/inbounds/setEnable/", a.entrypointSetEnable)
	mux.HandleFunc("/panel/api/inbounds/bulkDel", a.entrypointsBulkDelete)
	mux.HandleFunc("/panel/api/inbounds/:id/resetTraffic", a.resetTraffic)
	mux.HandleFunc("/panel/api/inbounds/resetAllTraffics", a.resetTraffic)

	mux.HandleFunc("/panel/api/clients/devices/block", a.deviceBlock)
	mux.HandleFunc("/panel/api/clients/devices/forget", a.deviceForget)
	mux.HandleFunc("/panel/api/clients/addresses/forget", a.addressForget)
	mux.HandleFunc("/panel/api/clients/exits/forget", a.exitForget)

	mux.HandleFunc("/panel/api/network/key", a.networkKey)

	mux.HandleFunc("/panel/api/backup/pull", a.getDB)
	mux.HandleFunc("/panel/api/backup/restore", a.importDB)

	for _, path := range []string{
		"/panel/api/publish/plan", "/panel/api/publish/apply",
		"/panel/api/publish/push", "/panel/api/publish/discard",
		"/panel/api/publish/status",
	} {
		mux.HandleFunc(path, a.draft)
	}

	mux.HandleFunc("/panel/api/clients/bulkAttach", a.attachIsByGroup)
	mux.HandleFunc("/panel/api/clients/bulkDetach", a.attachIsByGroup)
}

func (a *API) nodeProbe(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(lastPathSegment(r.URL.Path))
	if err != nil {
		sendFail(w, err)
		return
	}

	started := time.Now()
	body, err := a.fleet.Ask(id, "hello", nil)
	if err != nil {
		sendOK(w, map[string]any{"status": "offline", "error": err.Error()})
		return
	}

	var info struct {
		Version string `json:"version"`
	}
	json.Unmarshal(body, &info)
	sendOK(w, map[string]any{
		"status":    "online",
		"latencyMs": int(time.Since(started).Milliseconds()),
		"version":   info.Version,
	})
}

func (a *API) nodeSetEnable(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(lastPathSegment(r.URL.Path))
	if err != nil {
		sendFail(w, err)
		return
	}

	var body struct {
		Enable bool `json:"enable"`
	}
	bindBody(r, &body)

	rows, err := a.nodeRows()
	if err != nil {
		sendFail(w, err)
		return
	}
	for _, row := range rows {
		if int(numberOf(row["id"])) != id {
			continue
		}
		row["enable"] = body.Enable
		results, err := a.write("nodes.save", row)
		if err != nil {
			sendFailWith(w, err, results)
			return
		}
		sendOK(w, map[string]any{"nodes": results})
		return
	}
	sendFail(w, fmt.Errorf("no node %d", id))
}

func (a *API) nodeRows() ([]map[string]any, error) {
	body, err := a.fleet.Read("nodes.list", nil)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	err = json.Unmarshal(body, &rows)
	return rows, err
}

func (a *API) clientGet(w http.ResponseWriter, r *http.Request) {
	email := pathTail(r.URL.Path, "/panel/api/clients/get/")

	rows, _, err := a.readClients()
	if err != nil {
		sendFail(w, err)
		return
	}
	for _, row := range rows {
		if row["email"] != email {
			continue
		}
		entries, _ := row["inboundIds"].([]int)
		if entries == nil {
			entries = []int{}
		}
		sendOK(w, map[string]any{"client": row, "inboundIds": entries})
		return
	}
	sendFail(w, fmt.Errorf("no client named %q", email))
}

func (a *API) clientsBulkDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Emails []string `json:"emails"`
	}
	if err := bindBody(r, &body); err != nil {
		sendFail(w, err)
		return
	}

	byEmail, err := a.clientIDs()
	if err != nil {
		sendFail(w, err)
		return
	}

	deleted := 0
	skipped := []map[string]string{}
	for _, email := range body.Emails {
		id, known := byEmail[email]
		if !known {
			skipped = append(skipped, map[string]string{"email": email, "reason": "no such client"})
			continue
		}
		if _, err := a.write("clients.delete", map[string]int{"id": id}); err != nil {
			skipped = append(skipped, map[string]string{"email": email, "reason": err.Error()})
			continue
		}
		deleted++
	}
	sendOK(w, map[string]any{"deleted": deleted, "skipped": skipped})
}

func (a *API) clientsBulkCreate(w http.ResponseWriter, r *http.Request) {
	var rows []map[string]any
	if err := bindBody(r, &rows); err != nil {
		sendFail(w, err)
		return
	}

	created := 0
	skipped := []map[string]string{}
	for _, row := range rows {
		if nested, ok := row["client"].(map[string]any); ok {
			row = nested
		}
		if uuid := firstText(row["subId"], row["uuid"], row["id"]); uuid != "" {
			row["uuid"] = uuid
			delete(row, "id")
			delete(row, "subId")
		}
		if err := a.resolveGroup(row); err != nil {
			skipped = append(skipped, map[string]string{"email": textOf(row["email"]), "reason": err.Error()})
			continue
		}
		if _, err := a.write("clients.save", row); err != nil {
			skipped = append(skipped, map[string]string{"email": textOf(row["email"]), "reason": err.Error()})
			continue
		}
		created++
	}
	sendOK(w, map[string]any{"created": created, "skipped": skipped})
}

func (a *API) clientsBulkAdjust(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Emails  []string `json:"emails"`
		AddDays int      `json:"addDays"`
	}
	if err := bindBody(r, &body); err != nil {
		sendFail(w, err)
		return
	}

	rows, _, err := a.readClients()
	if err != nil {
		sendFail(w, err)
		return
	}

	wanted := map[string]bool{}
	for _, email := range body.Emails {
		wanted[email] = true
	}

	adjusted := 0
	skipped := []map[string]string{}
	for _, row := range rows {
		email := textOf(row["email"])
		if !wanted[email] {
			continue
		}

		expiry := int64(numberOf(row["expiryTime"]))
		now := time.Now().UnixMilli()
		if expiry < now {
			expiry = now
		}
		row["expiryTime"] = expiry + int64(body.AddDays)*24*60*60*1000

		if err := a.resolveGroup(row); err != nil {
			skipped = append(skipped, map[string]string{"email": email, "reason": err.Error()})
			continue
		}
		if _, err := a.write("clients.save", row); err != nil {
			skipped = append(skipped, map[string]string{"email": email, "reason": err.Error()})
			continue
		}
		adjusted++
	}
	sendOK(w, map[string]any{"adjusted": adjusted, "skipped": skipped})
}

func (a *API) clientsDelDepleted(w http.ResponseWriter, r *http.Request) {
	rows, _, err := a.readClients()
	if err != nil {
		sendFail(w, err)
		return
	}

	now := time.Now().UnixMilli()
	deleted := 0
	for _, row := range rows {
		expiry := int64(numberOf(row["expiryTime"]))
		if expiry <= 0 || expiry >= now {
			continue
		}
		id := int(numberOf(row["id"]))
		if _, err := a.write("clients.delete", map[string]int{"id": id}); err == nil {
			deleted++
		}
	}
	sendOK(w, map[string]any{"deleted": deleted})
}

func (a *API) groupBulkAdd(w http.ResponseWriter, r *http.Request) {
	a.moveToGroup(w, r, true)
}

func (a *API) groupBulkRemove(w http.ResponseWriter, r *http.Request) {
	a.moveToGroup(w, r, false)
}

func (a *API) moveToGroup(w http.ResponseWriter, r *http.Request, join bool) {
	var body struct {
		Emails []string `json:"emails"`
		Group  string   `json:"group"`
	}
	if err := bindBody(r, &body); err != nil {
		sendFail(w, err)
		return
	}

	rows, _, err := a.readClients()
	if err != nil {
		sendFail(w, err)
		return
	}

	wanted := map[string]bool{}
	for _, email := range body.Emails {
		wanted[email] = true
	}

	moved := 0
	for _, row := range rows {
		if !wanted[textOf(row["email"])] {
			continue
		}
		if join {
			row["group"] = body.Group
		} else {
			row["group"] = ""
		}
		if err := a.resolveGroup(row); err != nil {
			sendFail(w, err)
			return
		}
		if !join {
			row["groupId"] = 0
		}
		if _, err := a.write("clients.save", row); err != nil {
			sendFail(w, err)
			return
		}
		moved++
	}
	sendOK(w, map[string]any{"moved": moved})
}

func (a *API) groupDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		ID   int    `json:"id"`
	}
	if err := bindBody(r, &body); err != nil {
		sendFail(w, err)
		return
	}

	groups, err := a.groups()
	if err != nil {
		sendFail(w, err)
		return
	}

	target := -1
	name := body.Name
	for _, g := range groups {
		if g.ID == body.ID || (body.Name != "" && g.Name == body.Name) {
			target, name = g.ID, g.Name
			break
		}
	}
	if target < 0 {
		sendFail(w, fmt.Errorf("no group named %q", body.Name))
		return
	}

	rows, _, err := a.readClients()
	if err != nil {
		sendFail(w, err)
		return
	}

	affected := 0
	for _, row := range rows {
		if textOf(row["group"]) != name {
			continue
		}
		row["group"] = ""
		if err := a.resolveGroup(row); err != nil {
			sendFail(w, err)
			return
		}
		row["groupId"] = 0
		if _, err := a.write("clients.save", row); err != nil {
			sendFail(w, err)
			return
		}
		affected++
	}

	results, err := a.write("groups.delete", map[string]int{"id": target})
	if err != nil {
		sendFailWith(w, err, results)
		return
	}
	a.cache.forget()
	sendOK(w, map[string]any{"affected": affected, "nodes": results})
}

func (a *API) groupRename(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
		Name string `json:"name"`
		ID   int    `json:"id"`
	}
	if err := bindBody(r, &body); err != nil {
		sendFail(w, err)
		return
	}
	if body.To == "" {
		body.To = body.Name
	}

	groups, err := a.groups()
	if err != nil {
		sendFail(w, err)
		return
	}
	for _, g := range groups {
		if g.ID != body.ID && g.Name != body.From {
			continue
		}
		results, err := a.write("groups.save", map[string]any{
			"id": g.ID, "name": body.To, "entrypointIds": g.EntrypointIDs,
			"deviceLimit": g.DeviceLimit, "allowExit": g.AllowExit,
		})
		if err != nil {
			sendFailWith(w, err, results)
			return
		}
		sendOK(w, map[string]any{"nodes": results})
		return
	}
	sendFail(w, fmt.Errorf("no group named %q", body.From))
}

func (a *API) groupEntrypoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		groups, err := a.groups()
		if err != nil {
			sendFail(w, err)
			return
		}
		sendOK(w, groups)
		return
	}

	var body struct {
		ID            int    `json:"id"`
		Name          string `json:"name"`
		Group         string `json:"group"`
		EntrypointIDs []int  `json:"entrypointIds"`
		DeviceLimit   int    `json:"deviceLimit"`
		AllowExit     bool   `json:"allowExit"`
	}
	if err := bindBody(r, &body); err != nil {
		sendFail(w, err)
		return
	}
	if body.Name == "" {
		body.Name = body.Group
	}
	if body.EntrypointIDs == nil {
		body.EntrypointIDs = []int{}
	}

	groups, err := a.groups()
	if err != nil {
		sendFail(w, err)
		return
	}
	for _, g := range groups {
		if g.ID != body.ID && g.Name != body.Name {
			continue
		}
		results, err := a.write("groups.save", map[string]any{
			"id": g.ID, "name": g.Name, "entrypointIds": body.EntrypointIDs,
			"deviceLimit": body.DeviceLimit, "allowExit": body.AllowExit,
		})
		if err != nil {
			sendFailWith(w, err, results)
			return
		}
		sendOK(w, map[string]any{"nodes": results})
		return
	}
	sendFail(w, fmt.Errorf("no group %d", body.ID))
}

func (a *API) entrypointGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(lastPathSegment(r.URL.Path))
	if err != nil {
		sendFail(w, err)
		return
	}
	rows, err := a.entrypoints()
	if err != nil {
		sendFail(w, err)
		return
	}
	for _, row := range rows {
		if int(numberOf(row["id"])) == id {
			sendOK(w, row)
			return
		}
	}
	sendFail(w, fmt.Errorf("no entrypoint %d", id))
}

func (a *API) entrypointSetEnable(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(lastPathSegment(r.URL.Path))
	if err != nil {
		sendFail(w, err)
		return
	}

	var body struct {
		Enable bool `json:"enable"`
	}
	bindBody(r, &body)

	raw, err := a.fleet.Read("entrypoints.list", nil)
	if err != nil {
		sendFail(w, err)
		return
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		sendFail(w, err)
		return
	}

	for _, row := range rows {
		if int(numberOf(row["id"])) != id {
			continue
		}
		row["enable"] = body.Enable
		results, err := a.write("entrypoints.save", row)
		if err != nil {
			sendFailWith(w, err, results)
			return
		}
		sendOK(w, map[string]any{"nodes": results})
		return
	}
	sendFail(w, fmt.Errorf("no entrypoint %d", id))
}

func (a *API) entrypointsBulkDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int `json:"ids"`
	}
	if err := bindBody(r, &body); err != nil {
		sendFail(w, err)
		return
	}

	deleted := 0
	for _, id := range body.IDs {
		if _, err := a.write("entrypoints.delete", map[string]int{"id": id}); err == nil {
			deleted++
		}
	}
	sendOK(w, map[string]any{"deleted": deleted})
}

func (a *API) deviceBlock(w http.ResponseWriter, r *http.Request) {
	a.deviceOp(w, r, "devices.block")
}

func (a *API) deviceForget(w http.ResponseWriter, r *http.Request) {
	a.deviceOp(w, r, "devices.forget")
}

func (a *API) deviceOp(w http.ResponseWriter, r *http.Request, op string) {
	var body struct {
		Email       string `json:"email"`
		ClientID    int    `json:"clientId"`
		Fingerprint string `json:"fingerprint"`
		Blocked     bool   `json:"blocked"`
	}
	if err := bindBody(r, &body); err != nil {
		sendFail(w, err)
		return
	}
	if body.Fingerprint == "" {
		sendFail(w, fmt.Errorf("no device named"))
		return
	}

	if body.ClientID == 0 && body.Email != "" {
		byEmail, err := a.clientIDs()
		if err != nil {
			sendFail(w, err)
			return
		}
		body.ClientID = byEmail[body.Email]
	}
	if body.ClientID == 0 {
		sendFail(w, fmt.Errorf("no client named %q", body.Email))
		return
	}

	results, err := a.write(op, map[string]any{
		"clientId": body.ClientID, "fingerprint": body.Fingerprint, "blocked": body.Blocked,
	})
	if err != nil {
		sendFailWith(w, err, results)
		return
	}
	sendOK(w, map[string]any{"nodes": results})
}

func (a *API) networkKey(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"key": hex.EncodeToString(a.fleet.key[:])}

	if token := a.fleet.Token(); token != "" {
		out["adminUuid"] = token
		tag := a.fleet.Tag()
		if tag == "" {
			if rows, _, err := a.readClients(); err == nil {
				for _, row := range rows {
					if textOf(row["uuid"]) == token {
						tag = textOf(row["email"])
						break
					}
				}
			}
		}
		out["adminTag"] = tag
	}
	sendOK(w, out)
}

func (a *API) resetTraffic(w http.ResponseWriter, r *http.Request) {
	only := pathTail(r.URL.Path, "/panel/api/clients/resetTraffic/")

	rows, _, err := a.readClients()
	if err != nil {
		sendFail(w, err)
		return
	}

	wanted := []uint32{}
	for _, row := range rows {
		uuid := textOf(row["uuid"])
		if uuid == "" {
			continue
		}
		if only != "" && textOf(row["email"]) != only {
			continue
		}
		wanted = append(wanted, qdcrypt.SessionID(uuid))
	}
	if len(wanted) == 0 {
		sendFail(w, fmt.Errorf("no client named %q", only))
		return
	}

	for _, node := range a.fleet.Live() {
		if _, err := a.fleet.Ask(node.ID, "sessions.reset", map[string]any{"sessions": wanted}); err != nil {
			sendFail(w, err)
			return
		}
	}
	sendOK(w, map[string]any{"reset": len(wanted)})
}

func (a *API) attachIsByGroup(w http.ResponseWriter, r *http.Request) {
	sendFail(w, fmt.Errorf("entrypoints are attached to a group, not to one client — change the client's group instead"))
}

func (a *API) clientIDs() (map[string]int, error) {
	rows, _, err := a.readClients()
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[textOf(row["email"])] = int(numberOf(row["id"]))
	}
	return out, nil
}

func (a *API) resolveGroup(row map[string]any) error {
	name, present := row["group"].(string)
	if !present {
		return nil
	}
	delete(row, "group")
	if name == "" {
		return nil
	}

	ids, err := a.groupIDByName()
	if err != nil {
		return err
	}
	id, known := ids[name]
	if !known {
		return errUnknownGroup(name)
	}
	row["groupId"] = id
	return nil
}

func pathTail(path, prefix string) string {
	tail := strings.TrimPrefix(path, prefix)
	if decoded, err := url.PathUnescape(tail); err == nil {
		return decoded
	}
	return tail
}

func numberOf(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case uint:
		return float64(n)
	case uint32:
		return float64(n)
	case uint64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

func textOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func (a *API) addressForget(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		ClientID int    `json:"clientId"`
		IP       string `json:"ip"`
	}
	if err := bindBody(r, &body); err != nil {
		sendFail(w, err)
		return
	}

	if body.ClientID == 0 && body.Email != "" {
		byEmail, err := a.clientIDs()
		if err != nil {
			sendFail(w, err)
			return
		}
		body.ClientID = byEmail[body.Email]
	}
	if body.ClientID == 0 {
		sendFail(w, fmt.Errorf("no client named %q", body.Email))
		return
	}

	results, err := a.write("addresses.forget", map[string]any{
		"clientId": body.ClientID, "ip": body.IP,
	})
	if err != nil {
		sendFailWith(w, err, results)
		return
	}
	a.cache.forget()
	sendOK(w, map[string]any{"nodes": results})
}

func (a *API) exitForget(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		ClientID int    `json:"clientId"`
		NodeID   int    `json:"nodeId"`
	}
	if err := bindBody(r, &body); err != nil {
		sendFail(w, err)
		return
	}

	if body.ClientID == 0 && body.Email != "" {
		byEmail, err := a.clientIDs()
		if err != nil {
			sendFail(w, err)
			return
		}
		body.ClientID = byEmail[body.Email]
	}
	if body.ClientID == 0 {
		sendFail(w, fmt.Errorf("no client named %q", body.Email))
		return
	}

	results, err := a.write("exits.forget", map[string]any{
		"clientId": body.ClientID, "nodeId": body.NodeID,
	})
	if err != nil {
		sendFailWith(w, err, results)
		return
	}
	a.cache.forget()
	sendOK(w, map[string]any{"nodes": results})
}
