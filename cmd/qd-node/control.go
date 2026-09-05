//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/jaywehosl/quic-diver/internal/dnsproxy"
	"github.com/jaywehosl/quic-diver/internal/netstate"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
	"github.com/jaywehosl/quic-diver/internal/qsrv"
	"github.com/jaywehosl/quic-diver/internal/store"
)

type nodeInfo struct {
	ID        int     `json:"id"`
	Tag       string  `json:"tag"`
	Role      string  `json:"role"`
	Revision  int     `json:"revision"`
	UptimeSec int64   `json:"uptimeSecs"`
	Platform  string  `json:"platform"`
	Version   string  `json:"version"`
	CPUPct    float64 `json:"cpuPct"`
	MemPct    float64 `json:"memPct"`
	Carrying  int     `json:"carrying"`
}

type controlState struct {
	tag        string
	role       string
	address    string
	uuid       string
	enable     bool
	key        string
	id         int
	port       int
	db         *store.DB
	dbPath     string
	tookAt     time.Time
	tookChunks int
	gaveAt     time.Time
	gaveChunks int
	started    time.Time
	metrics    *metrics
	logs       *logRing
	restart    func()

	netKey   *qdcrypt.Key
	dnsUp    string
	dnsDown  string
	saidExit string

	node     *qsrv.Node
	sessions *sessionMap
	exits    int
	watch    *presence
	epoch    int64

	dns *dnsproxy.Resolver
}

var clientOps = map[string]bool{
	"whoami": true,
	"join":   true,
	"bye":    true,
	"dns":    true,
}

func (state *controlState) mayAdminister(auth string) bool {
	clients, err := state.db.Clients()
	if err != nil {
		return false
	}
	seen := false
	for _, c := range clients {
		if !c.Admin || !c.Enable {
			continue
		}
		seen = true
		if auth != "" && auth == c.UUID {
			return true
		}
	}
	return !seen
}

func handleControl(state *controlState, req request) response {
	if !clientOps[req.Op] && !state.mayAdminister(req.Auth) {
		return response{OK: false, Error: "not an administrator of this network"}
	}

	if writeOps[req.Op] {
		defer state.traceWrite(req)()
	}

	switch req.Op {
	case "hello":
		revision, _ := state.db.Version()
		var latest sample
		if state.metrics != nil {
			latest = state.metrics.Latest()
		}
		return reply(req, nodeInfo{
			CPUPct:    latest.CPU,
			MemPct:    latest.Mem,
			Carrying:  int(latest.Online),
			ID:        state.id,
			Tag:       state.tag,
			Role:      state.role,
			Revision:  revision,
			UptimeSec: int64(time.Since(state.started).Seconds()),
			Platform:  runtime.GOOS + "/" + runtime.GOARCH,
			Version:   version,
		})

	case "whoami":
		var body struct {
			Token string `json:"token"`
			deviceClaim
		}
		json.Unmarshal(req.Body, &body)
		if body.Token != "" && state.watch != nil {
			state.watch.Checked(qdcrypt.SessionID(body.Token), body.Fingerprint)
		}
		return reply(req, state.whoami(body.Token, body.deviceClaim))

	case "nodes.list":
		return read(req, state.db.Nodes)
	case "entrypoints.list":
		return read(req, state.db.Entrypoints)
	case "groups.list":
		return read(req, state.db.Groups)
	case "clients.list":
		return read(req, state.db.Clients)

	case "nodes.save":
		answer := state.wrote(merged(state, req, state.db.Nodes,
			func(n netstate.Node) int { return n.ID }, state.db.SaveNode), true)
		state.followPort()
		return answer
	case "entrypoints.save":
		return state.wrote(merged(state, req, state.db.Entrypoints,
			func(e netstate.Entrypoint) int { return e.ID }, state.db.SaveEntrypoint), true)
	case "groups.save":
		return state.wrote(merged(state, req, state.db.Groups,
			func(g netstate.Group) int { return g.ID }, state.db.SaveGroup), false)
	case "clients.save":
		var want struct {
			ID     int  `json:"id"`
			Admin  bool `json:"admin"`
			Enable bool `json:"enable"`
		}
		json.Unmarshal(req.Body, &want)
		if why := state.lastAdmin(want.ID, want.Admin && want.Enable); why != "" {
			return response{OK: false, Error: why}
		}
		return state.wrote(merged(state, req, state.db.Clients,
			func(c netstate.Client) int { return c.ID }, state.db.SaveClient), false)

	case "nodes.delete":
		return state.wrote(remove(state, req, state.db.DeleteNode), true)
	case "entrypoints.delete":
		return state.wrote(remove(state, req, state.db.DeleteEntrypoint), true)
	case "groups.delete":
		return state.wrote(remove(state, req, state.db.DeleteGroup), false)
	case "clients.delete":
		var going struct {
			ID int `json:"id"`
		}
		json.Unmarshal(req.Body, &going)
		if why := state.lastAdmin(going.ID, false); why != "" {
			return response{OK: false, Error: why}
		}
		return state.wrote(remove(state, req, state.db.DeleteClient), false)

	case "status":
		return reply(req, state.machineStatus())

	case "join":
		var joining struct {
			Token string `json:"token"`
			deviceClaim
		}
		json.Unmarshal(req.Body, &joining)
		if answer := state.whoami(joining.Token, joining.deviceClaim); answer["carried"] != true {
			return reply(req, answer)
		}
		if joining.Token != "" && state.watch != nil {
			state.watch.Joining(qdcrypt.SessionID(joining.Token), joining.Fingerprint)
		}
		return reply(req, map[string]bool{"joined": true})

	case "bye":
		var body struct {
			Token string `json:"token"`
		}
		json.Unmarshal(req.Body, &body)
		if body.Token != "" && state.watch != nil && state.servesClients() {
			state.watch.Leaving(qdcrypt.SessionID(body.Token))
		}
		return reply(req, map[string]bool{"gone": true})

	case "sessions.reset":
		var body struct {
			Sessions []uint32 `json:"sessions"`
		}
		json.Unmarshal(req.Body, &body)
		if state.sessions == nil || state.sessions.reset == nil {
			return response{OK: false, Error: "this node keeps no counters"}
		}
		clients, _ := state.db.Clients()
		owner := map[uint32]int{}
		for _, c := range clients {
			if c.UUID != "" {
				owner[qdcrypt.SessionID(c.UUID)] = c.ID
			}
		}
		for _, id := range body.Sessions {
			if err := state.sessions.reset(id); err != nil {
				return response{OK: false, Error: err.Error()}
			}
			if client, known := owner[id]; known {
				state.db.ResetTraffic(client)
			}
		}
		return reply(req, map[string]int{"reset": len(body.Sessions)})

	case "devices.block":
		var body struct {
			ClientID    int    `json:"clientId"`
			Fingerprint string `json:"fingerprint"`
			Blocked     bool   `json:"blocked"`
		}
		if err := json.Unmarshal(req.Body, &body); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		if err := state.db.BlockDevice(body.ClientID, body.Fingerprint, body.Blocked); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		state.bumpAfterWrite()
		return reply(req, map[string]bool{"blocked": body.Blocked})

	case "devices.forget":
		var body struct {
			ClientID    int    `json:"clientId"`
			Fingerprint string `json:"fingerprint"`
		}
		if err := json.Unmarshal(req.Body, &body); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		gone, err := state.db.ForgetDevice(body.ClientID, body.Fingerprint)
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		return reply(req, map[string]int{"forgotten": gone})

	case "exits.forget":
		var body struct {
			ClientID int `json:"clientId"`
			NodeID   int `json:"nodeId"`
		}
		if err := json.Unmarshal(req.Body, &body); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		gone, err := state.db.ForgetExit(body.ClientID, body.NodeID)
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		return reply(req, map[string]int{"forgotten": gone})

	case "addresses.forget":
		var body struct {
			ClientID int    `json:"clientId"`
			IP       string `json:"ip"`
		}
		if err := json.Unmarshal(req.Body, &body); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		gone, err := 0, error(nil)
		if body.IP == "" {
			gone, err = state.db.ForgetAddresses(body.ClientID)
		} else {
			gone, err = state.db.ForgetAddress(body.ClientID, body.IP)
		}
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		return reply(req, map[string]int{"forgotten": gone})

	case "network.settings":
		settings, err := state.db.NetworkSettings()
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		return reply(req, settings)

	case "network.settings.save":
		body, err := state.db.NetworkSettings()
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		was := body
		if err := json.Unmarshal(req.Body, &body); err != nil {
			return response{OK: false, Error: err.Error()}
		}

		if body == was {
			return reply(req, was)
		}

		if err := state.db.SaveNetworkSettings(body); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		state.bumpAfterWrite()
		state.reloadResolver()
		settings, err := state.db.NetworkSettings()
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		if state.node != nil {
			state.node.Retune(tunablesFrom(settings))
		}

		restarting := false
		if state.restart != nil && datapathMoved(was, settings) {
			restarting = true
			fmt.Printf("datapath   %s changed the listener, restarting\n", movedWhat(was, settings))
			go func() {
				time.Sleep(300 * time.Millisecond)
				state.restart()
			}()
		}

		said := map[string]any{"settings": settings, "restarting": restarting}
		if restarting {
			said["moved"] = movedWhat(was, settings)
		}
		return reply(req, said)

	case "dns":
		return state.resolve(req)

	case "dns.records":
		return read(req, state.db.DNSRecords)

	case "dns.records.save":
		var body store.DNSRecord
		if err := json.Unmarshal(req.Body, &body); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		id, err := state.db.SaveDNSRecord(body)
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		state.bumpAfterWrite()
		state.reloadResolver()
		return reply(req, map[string]int{"id": id})

	case "dns.records.delete":
		var body struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(req.Body, &body); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		if err := state.db.DeleteDNSRecord(body.ID); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		state.bumpAfterWrite()
		state.reloadResolver()
		return reply(req, map[string]int{"id": body.ID})

	case "dns.stats":
		if state.dns == nil {
			return reply(req, dnsproxy.Stats{})
		}
		return reply(req, state.dns.Stats())

	case "dns.flush":
		if state.dns != nil {
			state.dns.Flush()
		}
		return reply(req, map[string]bool{"flushed": true})

	case "clients.stats":
		traffic, err := state.db.Traffic()
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		addresses, err := state.db.Addresses()
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		devices, err := state.db.Devices()
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		carried, err := state.db.PeerTraffic()
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		mine, err := state.db.TrafficFrom(state.id)
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		exits, err := state.db.Exits()
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		return reply(req, map[string]any{
			"traffic": traffic, "mine": mine, "addresses": addresses, "devices": devices,
			"carried": carried, "exits": exits,
		})

	case "sessions":
		if state.sessions == nil || state.sessions.stat == nil {
			return reply(req, []sessionStat{})
		}
		stats, err := state.sessions.stat()
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		return reply(req, stats)

	case "history":
		var body struct {
			Key    string `json:"key"`
			Bucket int    `json:"bucket"`
			Window int    `json:"window"`
		}
		json.Unmarshal(req.Body, &body)
		if state.metrics == nil {
			return reply(req, []point{})
		}
		return reply(req, state.metrics.Series(body.Key, body.Bucket, body.Window))

	case "history.export":
		if state.metrics == nil {
			return reply(req, []sample{})
		}
		return reply(req, state.metrics.Export())

	case "logs":
		var body struct {
			Rows  int    `json:"rows"`
			Level string `json:"level"`
		}
		json.Unmarshal(req.Body, &body)
		if state.logs == nil {
			return reply(req, []string{})
		}
		return reply(req, state.logs.Tail(body.Rows, body.Level))

	case "logs.clear":
		var body struct {
			Level string `json:"level"`
		}
		json.Unmarshal(req.Body, &body)
		if state.logs == nil {
			return reply(req, map[string]int{"cleared": 0})
		}
		return reply(req, map[string]int{"cleared": state.logs.Forget(body.Level)})

	case "restart":
		if state.restart == nil {
			return response{OK: false, Error: "this node cannot restart itself"}
		}
		go func() {
			time.Sleep(300 * time.Millisecond)
			state.restart()
		}()
		return reply(req, map[string]bool{"restarting": true})

	case "db.get":
		return state.dbRead(req)

	case "db.put":
		return state.dbWrite(req)

	default:
		return response{OK: false, Error: fmt.Sprintf("unknown op %q", req.Op)}
	}
}

func datapathMoved(was, now store.NetworkSettings) bool {
	return was.StatsSeconds != now.StatsSeconds ||
		was.Pool != now.Pool || was.BrutalMbit != now.BrutalMbit ||
		was.SocketBuffer != now.SocketBuffer ||
		was.MaxStreams != now.MaxStreams ||
		was.StreamWindow != now.StreamWindow || was.MaxStreamWindow != now.MaxStreamWindow ||
		was.ConnWindow != now.ConnWindow || was.MaxConnWindow != now.MaxConnWindow ||
		was.IdleSeconds != now.IdleSeconds || was.KeepAliveSeconds != now.KeepAliveSeconds
}

func (state *controlState) servesClients() bool {
	return state.role != string(netstate.RoleEgress)
}

func (state *controlState) peerAddresses() []string {
	network, err := state.db.LoadState()
	if err != nil {
		return []string{}
	}

	seen := map[string]bool{}
	out := []string{}
	for _, n := range network.Nodes {
		if n.Address == "" || seen[n.Address] {
			continue
		}
		seen[n.Address] = true
		out = append(out, n.Address)
	}
	return out
}

func (state *controlState) reachableBy(c netstate.Client) []map[string]any {
	out := []map[string]any{}

	network, err := state.db.LoadState()
	if err != nil {
		return out
	}

	var wanted []int
	for _, g := range network.Groups {
		if g.ID == c.GroupID {
			wanted = g.EntrypointIDs
			break
		}
	}

	nodes := map[int]netstate.Node{}
	for _, n := range network.Nodes {
		nodes[n.ID] = n
	}

	for _, id := range wanted {
		for _, e := range network.Entrypoints {
			if e.ID != id || !e.Enable {
				continue
			}
			n, known := nodes[e.NodeID]
			if !known || !n.Enable || n.Address == "" || n.Role == netstate.RoleEgress {
				continue
			}
			out = append(out, map[string]any{
				"id": e.ID, "name": n.Tag, "address": n.Address, "port": e.Port,
			})
		}
	}
	return out
}

func (state *controlState) whoami(token string, claim deviceClaim) map[string]any {
	if token == "" {
		return map[string]any{"admin": false}
	}

	clients, err := state.db.Clients()
	if err != nil {
		return map[string]any{"admin": false}
	}

	anyAdmin := false
	for _, c := range clients {
		if c.Admin {
			anyAdmin = true
		}
		if c.UUID != token {
			continue
		}

		expired := c.ExpiryAt > 0 && c.ExpiryAt < time.Now().UnixMilli()
		answer := map[string]any{
			"allowExit":      state.mayExit(c),
			"refreshMinutes": state.refreshMinutes(),
			"fixedRate":      state.fixedRate(),
			"known":          true,
			"admin":          c.Admin && c.Enable,
			"tag":            c.Tag,
			"id":             c.ID,
			"enable":         c.Enable,
			"expired":        expired,
			"expiryTime":     c.ExpiryAt,
			"carried":        c.Enable && !expired && state.enable && state.servesClients() && state.carries(c),
		}
		answer["entrypoints"] = state.reachableBy(c)
		if c.Admin && c.Enable {
			answer["peers"] = state.peerAddresses()
		}
		if answer["carried"] == true {
			if seat := state.admit(c, claim); !seat.Allowed {
				answer["carried"] = false
				answer["refused"] = seat.Reason
			}
		} else if c.Enable && !expired {
			state.seeAgain(c, claim)
		}
		return answer
	}

	if !anyAdmin && token == state.key {
		return map[string]any{
			"known": true, "admin": true, "tag": "network key",
			"bootstrap": true, "enable": true, "carried": false,
		}
	}

	return map[string]any{"known": false, "admin": false, "carried": false}
}

func (state *controlState) wrote(answer response, self bool) response {
	if !answer.OK {
		return answer
	}
	if self {
		state.applySelf()
	}
	state.syncSessions()
	return answer
}

func (state *controlState) selfRow() (netstate.Node, bool) {
	nodes, err := state.db.Nodes()
	if err != nil {
		return netstate.Node{}, false
	}
	if state.uuid != "" {
		for _, n := range nodes {
			if n.UUID == state.uuid {
				return n, true
			}
		}
	}
	if state.address != "" {
		for _, n := range nodes {
			if n.Address == state.address {
				return n, true
			}
		}
	}
	for _, n := range nodes {
		if n.ID == state.id {
			return n, true
		}
	}
	return netstate.Node{}, false
}

func (state *controlState) applySelf() {
	n, found := state.selfRow()
	if !found {
		return
	}

	if n.Enable != state.enable {
		state.enable = n.Enable
		if n.Enable {
			fmt.Printf("carrying   on\n")
		} else {
			fmt.Printf("carrying   off — answers the panel and subscription checks, takes no client traffic\n")
		}
	}
	if n.ID != state.id {
		fmt.Printf("identity   this node is #%d now, was #%d\n", n.ID, state.id)
		state.id = n.ID
	}
	if n.Tag != state.tag {
		state.tag = n.Tag
	}
	if n.DNSPrimary != state.dnsUp || n.DNSSecondary != state.dnsDown {
		state.dnsUp, state.dnsDown = n.DNSPrimary, n.DNSSecondary
		state.reloadResolver()
		fmt.Printf("resolver   this node now answers from %s\n", state.upstreamsSaid())
	}
	if string(n.Role) != state.role && state.restart != nil {
		fmt.Printf("role       changed to %s, restarting\n", n.Role)
		go func() {
			time.Sleep(300 * time.Millisecond)
			state.restart()
		}()
	}
}

func (state *controlState) machineStatus() map[string]any {
	var s sample
	if state.metrics != nil {
		s = state.metrics.Latest()
	}
	memUsed, memTotal := uint64(0), uint64(0)
	swapUsed, swapTotal := uint64(0), uint64(0)
	if state.metrics != nil {
		mt, st := state.metrics.Totals()
		memTotal, swapTotal = uint64(mt), uint64(st)
		memUsed = uint64(mt * s.Mem / 100)
		swapUsed = uint64(st * s.Swap / 100)
	}
	diskUsed, diskTotal := diskBytes("/")

	return map[string]any{
		"cpu":        s.CPU,
		"cpuCores":   runtime.NumCPU(),
		"logicalPro": runtime.NumCPU(),
		"loads":      []float64{s.Load1, s.Load5, s.Load15},
		"mem":        map[string]any{"current": memUsed, "total": memTotal},
		"swap":       map[string]any{"current": swapUsed, "total": swapTotal},
		"disk":       map[string]any{"current": diskUsed, "total": diskTotal},
		"netIO":      map[string]any{"up": s.NetUp, "down": s.NetDown},
		"tcpCount":   int(s.TCPCount),
		"udpCount":   int(s.UDPCount),
		"uptime":     hostUptime(),
		"appUptime":  int64(time.Since(state.started).Seconds()),
		"online":     int(s.Online),
		"nodeId":     state.id,
		"tag":        state.tag,
	}
}

func read[T any](req request, list func() ([]T, error)) response {
	rows, err := list()
	if err != nil {
		return response{OK: false, Error: err.Error()}
	}
	return reply(req, rows)
}

func save[T any](state *controlState, req request, write func(T, int64) (int, error)) response {
	var row T
	if err := json.Unmarshal(req.Body, &row); err != nil {
		return response{OK: false, Error: err.Error()}
	}
	now := time.Now().UnixMilli()
	id, err := write(row, now)
	if err != nil {
		return response{OK: false, Error: err.Error()}
	}
	revision, _ := state.db.Settle(wanted(req), now)
	return reply(req, map[string]int{"id": id, "revision": revision})
}

func (state *controlState) followPort() {
	if state.restart == nil || state.id == 0 {
		return
	}
	rows, err := state.db.Nodes()
	if err != nil {
		return
	}
	for _, n := range rows {
		if n.ID != state.id || n.Port <= 0 || n.Port == state.port {
			continue
		}
		fmt.Printf("port       moving from %d to %d, restarting\n", state.port, n.Port)
		go func() {
			time.Sleep(300 * time.Millisecond)
			state.restart()
		}()
		return
	}
}

func merged[T any](state *controlState, req request, list func() ([]T, error),
	idOf func(T) int, write func(T, int64) (int, error)) response {

	var head struct {
		ID int `json:"id"`
	}
	json.Unmarshal(req.Body, &head)

	var row T
	var before []byte
	found := false
	if head.ID > 0 {
		rows, err := list()
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		for _, existing := range rows {
			if idOf(existing) == head.ID {
				before, _ = json.Marshal(existing)
				row, found = existing, true
				break
			}
		}
	}
	if err := json.Unmarshal(req.Body, &row); err != nil {
		return response{OK: false, Error: err.Error()}
	}

	after, _ := json.Marshal(row)
	if found && bytes.Equal(before, after) {
		revision, _ := state.db.Version()
		return reply(req, map[string]int{"id": head.ID, "revision": revision})
	}

	now := time.Now().UnixMilli()
	id, err := write(row, now)
	if err != nil {
		return response{OK: false, Error: err.Error()}
	}
	revision, _ := state.db.Settle(wanted(req), now)
	return reply(req, map[string]int{"id": id, "revision": revision})
}

func remove(state *controlState, req request, drop func(int) error) response {
	var body struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return response{OK: false, Error: err.Error()}
	}
	if err := drop(body.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		return response{OK: false, Error: err.Error()}
	}
	revision, _ := state.db.Settle(wanted(req), time.Now().UnixMilli())
	return reply(req, map[string]int{"id": body.ID, "revision": revision})
}

func wanted(req request) int {
	var head struct {
		Revision int `json:"revision"`
	}
	json.Unmarshal(req.Body, &head)
	return head.Revision
}

func reply(req request, body any) response {
	encoded, err := json.Marshal(body)
	if err != nil {
		return response{OK: false, Error: err.Error()}
	}
	return response{OK: true, Body: encoded}
}

func (state *controlState) bumpAfterWrite() {
	state.db.Bump(time.Now().UnixMilli())
}

func (state *controlState) refreshMinutes() int {
	settings, err := state.db.NetworkSettings()
	if err != nil {
		return 60
	}
	return settings.RefreshMinutes
}

func (state *controlState) fixedRate() int {
	settings, err := state.db.NetworkSettings()
	if err != nil {
		return 0
	}
	return settings.BrutalMbit
}

func (state *controlState) mySession() uint32 {
	if state.uuid == "" {
		return 0
	}
	return qdcrypt.SessionID(state.uuid)
}

var writeOps = map[string]bool{
	"nodes.save": true, "nodes.delete": true,
	"entrypoints.save": true, "entrypoints.delete": true,
	"groups.save": true, "groups.delete": true,
	"clients.save": true, "clients.delete": true,
	"network.settings.save": true,
	"dns.records.save":      true, "dns.records.delete": true,
	"devices.block":  true,
	"sessions.reset": true, "db.put": true,
}

func (state *controlState) traceWrite(req request) func() {
	was, _ := state.db.Version()
	began := time.Now()
	size := len(req.Body)

	return func() {
		now, _ := state.db.Version()
		took := time.Since(began)
		if now != was {
			fmt.Printf("write      %-22s body %5d B  revision %d -> %d  in %s\n",
				req.Op, size, was, now, took.Round(time.Millisecond))
			return
		}
		fmt.Printf("write      %-22s body %5d B  revision %d (unchanged)  in %s\n",
			req.Op, size, was, took.Round(time.Millisecond))
	}
}

func movedWhat(was, now store.NetworkSettings) string {
	moved := []string{}
	for _, item := range []struct {
		name string
		same bool
	}{
		{"stats_seconds", was.StatsSeconds == now.StatsSeconds},
		{"pool", was.Pool == now.Pool},
		{"brutal_mbit", was.BrutalMbit == now.BrutalMbit},
		{"socket_buffer", was.SocketBuffer == now.SocketBuffer},
		{"max_streams", was.MaxStreams == now.MaxStreams},
		{"stream_window", was.StreamWindow == now.StreamWindow},
		{"max_stream_window", was.MaxStreamWindow == now.MaxStreamWindow},
		{"conn_window", was.ConnWindow == now.ConnWindow},
		{"max_conn_window", was.MaxConnWindow == now.MaxConnWindow},
		{"idle_seconds", was.IdleSeconds == now.IdleSeconds},
		{"keepalive_seconds", was.KeepAliveSeconds == now.KeepAliveSeconds},
	} {
		if !item.same {
			moved = append(moved, item.name)
		}
	}
	if len(moved) == 0 {
		return "nothing"
	}
	return strings.Join(moved, ", ")
}

func askNode(state *controlState, op string, body []byte, auth string) (any, error) {
	answer := handleControl(state, request{Op: op, Body: body, Auth: auth})
	if !answer.OK {
		return nil, errors.New(answer.Error)
	}
	if len(answer.Body) == 0 {
		return nil, nil
	}
	return json.RawMessage(answer.Body), nil
}

func (state *controlState) carries(c netstate.Client) bool {
	network, err := state.db.LoadState()
	if err != nil {
		return false
	}

	var wanted []int
	for _, g := range network.Groups {
		if g.ID == c.GroupID {
			wanted = g.EntrypointIDs
			break
		}
	}

	for _, id := range wanted {
		for _, e := range network.Entrypoints {
			if e.ID == id && e.Enable && e.NodeID == state.id {
				return true
			}
		}
	}
	return false
}

func (state *controlState) lastAdmin(id int, stays bool) string {
	clients, err := state.db.Clients()
	if err != nil {
		return ""
	}

	left := 0
	touched := false
	for _, c := range clients {
		if c.ID == id {
			touched = true
			if stays {
				left++
			}
			continue
		}
		if c.Admin && c.Enable {
			left++
		}
	}
	if !touched || left > 0 {
		return ""
	}
	return "this is the only administrator left — the network would have nobody to manage it"
}

type request struct {
	Op   string
	Body []byte
	Auth string
}

type response struct {
	OK    bool
	Error string
	Body  []byte
}

const dbChunk = 1 << 20
