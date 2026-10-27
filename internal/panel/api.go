package panel

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/jaywehosl/qd/internal/netstate"
	"github.com/jaywehosl/qd/internal/qdcrypt"
)

type Prefs interface {
	Value(key string) (string, error)
	SetValue(key, value string) error
}

type API struct {
	fleet    *Fleet
	prefs    Prefs
	cache    *cache
	revision atomic.Int64
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

	mux.HandleFunc("/panel/api/clients/groups", a.groupsList)
	mux.HandleFunc("/panel/api/clients/groups/save", a.save("groups.save"))
	mux.HandleFunc("/panel/api/clients/groups/delete", a.groupDelete)

	mux.HandleFunc("/panel/api/clients/list", a.clientsList)
	mux.HandleFunc("/panel/api/clients/list/paged", a.clientsPaged)
	mux.HandleFunc("/panel/api/clients/add", a.clientAdd)
	mux.HandleFunc("/panel/api/clients/update/", a.clientUpdate)
	mux.HandleFunc("/panel/api/clients/del/", a.clientDelete)

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

func (a *API) remove(op string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(lastPathSegment(r.URL.Path))
		if err != nil {
			sendFail(w, err)
			return
		}

		skip := 0
		var off *WriteResult
		if op == "nodes.delete" {
			skip = n
			off = &WriteResult{NodeID: n, Tag: a.fleet.TagOf(n), OK: true}
			if _, err := a.fleet.Ask(n, "nodes.save", map[string]any{"id": n, "enable": false}); err != nil {
				off.OK, off.Error = false, "did not take its switch-off and keeps carrying until it is switched off by hand: "+err.Error()
			}
		}
		results, err := a.writeExcept(op, map[string]int{"id": n}, skip)
		if off != nil {
			results = append(results, *off)
		}
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
	row := map[string]any{}
	if err := bindBody(r, &row); err != nil {
		sendFail(w, err)
		return
	}
	delete(row, "id")
	if id > 0 {
		row["id"] = id
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
		Tag  string `json:"tag"`
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
	top := 0
	for _, h := range health {
		top = max(top, h.Revision)
	}
	if a.revision.Swap(int64(top)) != int64(top) {
		a.cache.forget()
	}

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

	nodeOfEntry := map[int]int{}
	for _, e := range entries {
		nodeOfEntry[int(numberOf(e["id"]))] = int(numberOf(e["nodeId"]))
	}

	groupExit := map[string]bool{}
	if groups, err := a.groups(); err == nil {
		for _, g := range groups {
			groupExit[g.Tag] = g.AllowExit
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
		if ids, ok := c["entrypointIds"].([]int); ok {
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
		row["clientCount"] = reach[id]
		row["onlineCount"] = online[id]
	}
}

func bindBody(r *http.Request, dst any) error {
	err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(dst)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
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
