package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
)

var networkKeys = []string{
	"refreshMinutes",
	"dnsPrimary",
	"dnsSecondary",
	"dnsCache",
	"dnsMinTtl",
	"dnsMaxTtl",
	"dnsStale",
	"mtu",
	"statsSeconds",
	"pool",
	"brutalMbit",
	"maxStreams",
	"streamWindowKb",
	"maxStreamWindowKb",
	"connWindowKb",
	"maxConnWindowKb",
	"idleSeconds",
	"keepAliveSeconds",
	"socketBufferKb",
}

func (a *API) dnsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/panel/api/dns/records", a.dnsRecords)
	mux.HandleFunc("/panel/api/dns/records/save", a.dnsRecordSave)
	mux.HandleFunc("/panel/api/dns/records/del", a.dnsRecordDelete)
	mux.HandleFunc("/panel/api/dns/stats", a.dnsStats)
	mux.HandleFunc("/panel/api/dns/flush", a.dnsFlush)
}

func (a *API) dnsRecords(w http.ResponseWriter, r *http.Request) {
	body, err := a.fleet.Read("dns.records", nil)
	if err != nil {
		sendFail(w, err)
		return
	}
	raw(w, body)
}

func (a *API) dnsRecordSave(w http.ResponseWriter, r *http.Request) {
	var row map[string]any
	if err := bindBody(r, &row); err != nil {
		sendFail(w, err)
		return
	}
	if row == nil {
		row = map[string]any{}
	}
	if suffix, _ := row["suffix"].(string); suffix == "" {
		sendFail(w, fmt.Errorf("a record needs a name"))
		return
	}
	if _, carried := row["enable"]; !carried {
		row["enable"] = true
	}

	results, err := a.write("dns.records.save", row)
	if err != nil {
		sendFailWith(w, err, results)
		return
	}
	sendOK(w, row)
}

func (a *API) dnsRecordDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID int `json:"id"`
	}
	if err := bindBody(r, &body); err != nil {
		sendFail(w, err)
		return
	}
	if body.ID == 0 {
		sendFail(w, fmt.Errorf("which record?"))
		return
	}

	results, err := a.write("dns.records.delete", map[string]any{"id": body.ID})
	if err != nil {
		sendFailWith(w, err, results)
		return
	}
	sendOK(w, map[string]int{"id": body.ID})
}

func (a *API) dnsStats(w http.ResponseWriter, r *http.Request) {
	out := []map[string]any{}
	for _, node := range a.fleet.Live() {
		body, err := a.fleet.Ask(node.ID, "dns.stats", nil)
		if err != nil {
			continue
		}
		var stats map[string]any
		if json.Unmarshal(body, &stats) != nil {
			continue
		}
		stats["nodeId"] = node.ID
		stats["tag"] = node.Tag
		out = append(out, stats)
	}
	sendOK(w, out)
}

func (a *API) dnsFlush(w http.ResponseWriter, r *http.Request) {
	results, err := a.write("dns.flush", nil)
	if err != nil {
		sendFailWith(w, err, results)
		return
	}
	sendOK(w, map[string]bool{"flushed": true})
}
