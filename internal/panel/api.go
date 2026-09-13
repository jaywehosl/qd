package panel

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/jaywehosl/quic-diver/internal/netstate"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
)

type Prefs interface {
	Value(key string) (string, error)
	SetValue(key, value string) error
}

type API struct {
	fleet *Fleet
	prefs Prefs
	cache *cache
}

func NewAPI(fleet *Fleet, prefs Prefs) *API {
	return &API{fleet: fleet, prefs: prefs, cache: newCache()}
}

func (a *API) write(op string, body any) ([]WriteResult, error) {
	return a.writeExcept(op, body, 0)
}

func (a *API) writeExcept(op string, body any, skip int) ([]WriteResult, error) {
	results, err := a.fleet.WriteExcept(op, body, skip)
	a.cache.forget()
	return results, err
}

func (a *API) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/panel/api/nodes/list", a.nodesList)
	mux.HandleFunc("/panel/api/nodes/names", a.nodeNames)
	mux.HandleFunc("/panel/api/nodes/add", a.addNode)
	mux.HandleFunc("/panel/api/nodes/update/", a.updateNode)
	mux.HandleFunc("/panel/api/nodes/del/", a.remove("nodes.delete"))

	mux.HandleFunc("/panel/api/inbounds/list", a.entrypointsList)
	mux.HandleFunc("/panel/api/inbounds/list/slim", a.entrypointsList)
	mux.HandleFunc("/panel/api/inbounds/add", a.save("entrypoints.save"))
	mux.HandleFunc("/panel/api/inbounds/update/", a.saveWithID("entrypoints.save"))
	mux.HandleFunc("/panel/api/inbounds/del/", a.remove("entrypoints.delete"))

	mux.HandleFunc("/panel/api/clients/groups", a.groupsList)
	mux.HandleFunc("/panel/api/clients/groups/create", a.save("groups.save"))
	mux.HandleFunc("/panel/api/clients/groups/delete", a.groupDelete)

	mux.HandleFunc("/panel/api/clients/list", a.clientsList)
	mux.HandleFunc("/panel/api/clients/list/paged", a.clientsPaged)
	mux.HandleFunc("/panel/api/clients/add", a.clientSave(0))
	mux.HandleFunc("/panel/api/clients/update/", a.clientSaveWithID)
	mux.HandleFunc("/panel/api/clients/del/", a.clientDelete)

	mux.HandleFunc("/panel/api/network/health", a.health)

	a.extraRoutes(mux)
	a.opsRoutes(mux)
	a.restRoutes(mux)
}

func (a *API) save(op string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var row map[string]any
		if err := bindBody(r, &row); err != nil {
			sendFail(w, err)
			return
		}
		results, err := a.write(op, row)
		if err != nil {
			sendFailWith(w, err, results)
			return
		}
		sendOK(w, map[string]any{"nodes": results})
	}
}

func (a *API) saveWithID(op string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := lastPathSegment(r.URL.Path)

		var row map[string]any
		if err := bindBody(r, &row); err != nil {
			sendFail(w, err)
			return
		}
		if row == nil {
			row = map[string]any{}
		}
		if n, err := strconv.Atoi(id); err == nil {
			row["ID"] = n
		}

		results, err := a.write(op, row)
		if err != nil {
			sendFailWith(w, err, results)
			return
		}
		sendOK(w, map[string]any{"nodes": results})
	}
}

func (a *API) remove(op string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := lastPathSegment(r.URL.Path)
		n, err := strconv.Atoi(id)
		if err != nil {
			var body struct {
				ID int `json:"id"`
			}
			bindBody(r, &body)
			n = body.ID
		}

		skip := 0
		if op == "nodes.delete" {
			skip = n
		}
		results, err := a.writeExcept(op, map[string]int{"id": n}, skip)
		if err != nil {
			sendFailWith(w, err, results)
			return
		}
		sendOK(w, map[string]any{"nodes": results})
	}
}

func (a *API) addNode(w http.ResponseWriter, r *http.Request) {
	a.writeNode(w, r, 0)
}

func (a *API) updateNode(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(lastPathSegment(r.URL.Path))
	a.writeNode(w, r, id)
}

func (a *API) writeNode(w http.ResponseWriter, r *http.Request, id int) {
	var row map[string]any
	if err := bindBody(r, &row); err != nil {
		sendFail(w, err)
		return
	}
	if row == nil {
		row = map[string]any{}
	}
	if id > 0 {
		row["ID"] = id
	}

	if text, _ := row["uuid"].(string); text == "" {
		row["uuid"] = a.nodeSecret(id)
	}

	results, err := a.write("nodes.save", row)
	if err != nil {
		sendFailWith(w, err, results)
		return
	}

	a.seedEntrypoints()
	sendOK(w, map[string]any{"nodes": results})
}

func (a *API) seedEntrypoints() {
	body, err := a.fleet.Read("nodes.list", nil)
	if err != nil {
		return
	}
	var nodes []struct {
		ID   int    `json:"id"`
		Tag  string `json:"name"`
		Port int    `json:"port"`
	}
	if json.Unmarshal(body, &nodes) != nil {
		return
	}

	entries, err := a.entrypoints()
	if err != nil {
		return
	}
	held := map[int]bool{}
	for _, e := range entries {
		held[int(numberOf(e["nodeId"]))] = true
	}

	for _, n := range nodes {
		if n.ID == 0 || n.Port == 0 || held[n.ID] {
			continue
		}
		a.write("entrypoints.save", map[string]any{
			"nodeId": n.ID, "port": n.Port, "remark": n.Tag, "enable": true,
		})
		held[n.ID] = true
	}
}

func (a *API) nodeSecret(id int) string {
	if id > 0 {
		if body, err := a.fleet.Read("nodes.list", nil); err == nil {
			var rows []struct {
				ID   int    `json:"id"`
				UUID string `json:"uuid"`
			}
			if json.Unmarshal(body, &rows) == nil {
				for _, n := range rows {
					if n.ID == id && n.UUID != "" {
						return n.UUID
					}
				}
			}
		}
	}
	return newUUID()
}

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (a *API) nodesList(w http.ResponseWriter, r *http.Request) {
	sendOK(w, a.nodeRowsWithCounts())
}

func (a *API) nodeNames(w http.ResponseWriter, r *http.Request) {
	sendOK(w, map[string]any{
		"ingress": netstate.PoolFor(netstate.RoleIngress),
		"egress":  netstate.PoolFor(netstate.RoleEgress),
	})
}

func (a *API) nodeRowsWithCounts() []map[string]any {
	health := a.fleet.Health()

	rows := make([]map[string]any, 0, len(health))
	for _, h := range health {
		blob, err := json.Marshal(h)
		if err != nil {
			continue
		}
		var row map[string]any
		if json.Unmarshal(blob, &row) != nil {
			continue
		}
		rows = append(rows, row)
	}

	a.countPerNode(rows)
	return rows
}

func (a *API) countPerNode(rows []map[string]any) {
	clients, _, err := a.readClients()
	if err != nil {
		return
	}
	entries, err := a.entrypoints()
	if err != nil {
		return
	}

	entriesOn := map[int]int{}
	nodeOfEntry := map[int]int{}
	for _, e := range entries {
		node := int(numberOf(e["nodeId"]))
		entriesOn[node]++
		nodeOfEntry[int(numberOf(e["id"]))] = node
	}

	groupExit := map[string]bool{}
	if groups, err := a.groups(); err == nil {
		for _, g := range groups {
			groupExit[g.Name] = g.AllowExit
		}
	}
	exits := []int{}
	for _, row := range rows {
		if textOf(row["role"]) == string(netstate.RoleEgress) {
			exits = append(exits, int(numberOf(row["id"])))
		}
	}
	live := a.sessions()

	reach := map[int]int{}
	online := map[int]int{}
	for _, c := range clients {
		here := map[int]bool{}
		if ids, ok := c["inboundIds"].([]int); ok {
			for _, id := range ids {
				here[nodeOfEntry[id]] = true
			}
		}
		client := netstate.Client{AllowExit: int(numberOf(c["allowExit"]))}
		if len(here) > 0 && client.MayExit(&netstate.Group{AllowExit: groupExit[textOf(c["group"])]}) {
			for _, id := range exits {
				here[id] = true
			}
		}
		on := live[qdcrypt.SessionID(textOf(c["uuid"]))].On
		for node := range here {
			reach[node]++
			if on[node] {
				online[node]++
			}
		}
	}

	for _, row := range rows {
		id := int(numberOf(row["id"]))
		row["inboundCount"] = entriesOn[id]
		row["clientCount"] = reach[id]
		row["onlineCount"] = online[id]
	}
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	sendOK(w, a.fleet.Health())
}

func bindBody(r *http.Request, dst any) error {
	kind := r.Header.Get("Content-Type")

	if strings.Contains(kind, "json") {
		return json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(dst)
	}

	if err := r.ParseForm(); err != nil {
		return err
	}

	fields := map[string]any{}
	for name, values := range r.PostForm {
		if len(values) == 0 {
			continue
		}
		fields[name] = typed(values[0])
	}

	blob, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return json.Unmarshal(blob, dst)
}

func typed(value string) any {
	switch value {
	case "true":
		return true
	case "false":
		return false
	}
	if n, err := strconv.ParseFloat(value, 64); err == nil {
		return n
	}
	return value
}

func lastPathSegment(path string) string {
	trimmed := strings.TrimSuffix(path, "/")
	if i := strings.LastIndexByte(trimmed, '/'); i >= 0 {
		return trimmed[i+1:]
	}
	return trimmed
}

func sendOK(w http.ResponseWriter, obj any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true, "msg": "", "obj": obj})
}

func raw(w http.ResponseWriter, obj json.RawMessage) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true, "msg": "", "obj": obj})
}

func sendFail(w http.ResponseWriter, err error) {
	sendFailWith(w, err, nil)
}

func sendFailWith(w http.ResponseWriter, err error, results []WriteResult) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"msg":     err.Error(),
		"obj":     map[string]any{"nodes": results},
	})
}
