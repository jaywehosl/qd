package panel

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jaywehosl/qd/internal/qdcrypt"
	"github.com/jaywehosl/qd/internal/update"
)

func (a *API) restRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/panel/api/nodes/probe/", a.nodeProbe)
	mux.HandleFunc("/panel/api/nodes/setEnable/", a.nodeSetEnable)

	mux.HandleFunc("/panel/api/clients/resetTraffic/", a.resetTraffic)
	mux.HandleFunc("/panel/api/clients/groups/members", a.groupMembers)

	mux.HandleFunc("/panel/api/clients/devices/block", a.clientOp("devices.block"))
	mux.HandleFunc("/panel/api/clients/devices/forget", a.clientOp("devices.forget"))
	mux.HandleFunc("/panel/api/clients/addresses/forget", a.clientOp("addresses.forget"))
	mux.HandleFunc("/panel/api/clients/exits/forget", a.clientOp("exits.forget"))

	mux.HandleFunc("/panel/api/network/key", a.networkKey)
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
	if err := bindBody(r, &body); err != nil {
		sendFail(w, err)
		return
	}

	results, err := a.write("nodes.save", map[string]any{"id": id, "enable": body.Enable})
	if err != nil {
		sendFailWith(w, err, results)
		return
	}
	sendOK(w, map[string]any{"nodes": results})
}

func (a *API) groupDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID int `json:"id"`
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
	ids, err := a.groupIDByTag()
	if err != nil {
		sendFail(w, err)
		return
	}

	affected := 0
	for _, row := range rows {
		if ids[textOf(row["group"])] != body.ID {
			continue
		}
		if _, err := a.write("clients.save", map[string]any{"id": row["id"], "groupId": 0}); err != nil {
			sendFail(w, err)
			return
		}
		affected++
	}

	results, err := a.write("groups.delete", map[string]int{"id": body.ID})
	if err != nil {
		sendFailWith(w, err, results)
		return
	}
	sendOK(w, map[string]any{"affected": affected, "nodes": results})
}

func (a *API) groupMembers(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Group   int   `json:"group"`
		Clients []int `json:"clients"`
	}
	if err := bindBody(r, &body); err != nil {
		sendFail(w, err)
		return
	}

	for _, id := range body.Clients {
		if _, err := a.write("clients.save", map[string]any{"id": id, "groupId": body.Group}); err != nil {
			sendFail(w, err)
			return
		}
	}
	sendOK(w, map[string]any{"moved": len(body.Clients)})
}

func (a *API) clientOp(op string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{}
		if err := bindBody(r, &body); err != nil {
			sendFail(w, err)
			return
		}
		if numberOf(body["clientId"]) <= 0 {
			sendFail(w, fmt.Errorf("no client id"))
			return
		}

		results, err := a.write(op, body)
		if err != nil {
			sendFailWith(w, err, results)
			return
		}
		sendOK(w, map[string]any{"nodes": results})
	}
}

func (a *API) networkKey(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"key": hex.EncodeToString(a.fleet.key[:]), "version": update.Version}

	if token := a.fleet.Token(); token != "" {
		out["adminUuid"] = token
		tag := a.fleet.Tag()
		if tag == "" {
			if rows, _, err := a.readClients(); err == nil {
				for _, row := range rows {
					if textOf(row["uuid"]) == token {
						tag = textOf(row["tag"])
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
	id, _ := strconv.Atoi(lastPathSegment(r.URL.Path))

	rows, _, err := a.readClients()
	if err != nil {
		sendFail(w, err)
		return
	}

	wanted := []uint32{}
	for _, row := range rows {
		if int(numberOf(row["id"])) == id && textOf(row["uuid"]) != "" {
			wanted = append(wanted, qdcrypt.SessionID(textOf(row["uuid"])))
		}
	}
	if len(wanted) == 0 {
		sendFail(w, fmt.Errorf("no client %d", id))
		return
	}

	for _, node := range a.fleet.Live() {
		if _, err := a.fleet.Ask(node.ID, "sessions.reset", map[string]any{"sessions": wanted}); err != nil {
			sendFail(w, err)
			return
		}
	}
	a.cache.forget()
	sendOK(w, map[string]any{"reset": len(wanted)})
}

func numberOf(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case uint32:
		return float64(n)
	case uint64:
		return float64(n)
	}
	return 0
}

func textOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func lastPathSegment(path string) string {
	trimmed := strings.TrimSuffix(path, "/")
	if i := strings.LastIndexByte(trimmed, '/'); i >= 0 {
		return trimmed[i+1:]
	}
	return trimmed
}
