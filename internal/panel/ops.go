package panel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// dbChunk — кусок базы за один запрос.
const dbChunk = 1 << 20

func (a *API) opsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/panel/api/nodes/restartAll", a.restartAll)
	mux.HandleFunc("/panel/api/nodes/", a.nodeScoped)

	mux.HandleFunc("/panel/api/server/getDb", a.getDB)
	mux.HandleFunc("/panel/api/server/importDB", a.importDB)
	mux.HandleFunc("/panel/api/server/sync", a.syncNetwork)
}

// restartAll просит узлы перезапуститься. Оборванный ответ здесь — не отказ:
// узел закрывает соединение ровно потому, что делает то, о чём попросили.
func (a *API) restartAll(w http.ResponseWriter, r *http.Request) {
	results, err := a.write("restart", nil)
	if len(results) == 0 {
		sendFail(w, err)
		return
	}
	for i := range results {
		if !results[i].OK && brokeOffRestarting(results[i].Error) {
			results[i].OK = true
			results[i].Error = ""
			results[i].Restarting = true
		}
	}
	for _, one := range results {
		if !one.OK {
			sendFailWith(w, fmt.Errorf("%s did not take the restart: %s", one.Tag, one.Error), results)
			return
		}
	}
	sendOK(w, map[string]any{"nodes": results})
}

// brokeOffRestarting — обрыв соединения посреди ответа. Узел уже подменил свой
// образ, договорить ему нечем, и жаловаться тут не на что.
func brokeOffRestarting(text string) bool {
	for _, mark := range []string{
		"H3_NO_ERROR", "H3x0", "close", "closed", "EOF",
		"connection reset", "no recent network activity", "server closed",
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(mark)) {
			return true
		}
	}
	return false
}

func (a *API) nodeScoped(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/panel/api/nodes/"), "/"), "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}

	switch parts[1] {
	case "logs":
		if len(parts) > 2 && parts[2] == "clear" {
			var want struct {
				Level string `json:"level"`
			}
			bindBody(r, &want)

			body, err := a.fleet.Ask(id, "logs.clear", map[string]any{"level": want.Level})
			if err != nil {
				sendFail(w, err)
				return
			}
			raw(w, body)
			return
		}

		download := len(parts) > 2 && parts[2] == "download"

		rows := 200
		if download {
			rows = 0
		} else if len(parts) > 2 {
			rows = atoiOr(parts[2], rows)
		}

		var want struct {
			Level string `json:"level"`
		}
		bindBody(r, &want)

		body, err := a.fleet.Ask(id, "logs", map[string]any{"rows": rows, "level": want.Level})
		if err != nil {
			sendFail(w, err)
			return
		}

		var lines []string
		json.Unmarshal(body, &lines)
		for i, line := range lines {
			lines[i] = toLocalTime(line)
		}

		if download {
			sendOK(w, strings.Join(lines, "\n"))
			return
		}
		sendOK(w, lines)

	case "history":
		if len(parts) > 2 && parts[2] == "export" {
			body, err := a.fleet.Ask(id, "history.export", nil)
			if err != nil {
				sendFail(w, err)
				return
			}
			raw(w, body)
			return
		}
		if len(parts) < 4 {
			http.NotFound(w, r)
			return
		}
		body, err := a.fleet.Ask(id, "history", map[string]any{
			"key":    parts[2],
			"bucket": atoiOr(parts[3], 2),
			"window": atoiOr(r.URL.Query().Get("window"), 0),
		})
		if err != nil {
			sendFail(w, err)
			return
		}
		raw(w, body)

	default:
		http.NotFound(w, r)
	}
}

const logStamp = "2006/01/02 15:04:05"

func toLocalTime(line string) string {
	if len(line) < len(logStamp) {
		return line
	}
	stamped, err := time.ParseInLocation(logStamp, line[:len(logStamp)], time.UTC)
	if err != nil {
		return line
	}
	return stamped.Local().Format(logStamp) + line[len(logStamp):]
}

type dbChunkReply struct {
	Size  int64  `json:"size"`
	Chunk []byte `json:"chunk"`
	EOF   bool   `json:"eof"`
	Sum   string `json:"sum"`
}

func sumOf(blob []byte) string {
	digest := sha256.Sum256(blob)
	return hex.EncodeToString(digest[:])
}

func (a *API) freshest() (int, error) {
	health := a.fleet.Health()
	best, revision := 0, -1
	for _, h := range health {
		if h.Online && h.Revision > revision {
			best, revision = h.ID, h.Revision
		}
	}
	if revision < 0 {
		return 0, fmt.Errorf("no node answered")
	}
	return best, nil
}

func (a *API) pullDB(id int) ([]byte, error) {
	var out []byte
	for {
		body, err := a.fleet.Ask(id, "db.get", map[string]int64{"offset": int64(len(out))})
		if err != nil {
			return nil, err
		}
		var reply dbChunkReply
		if err := json.Unmarshal(body, &reply); err != nil {
			return nil, err
		}
		out = append(out, reply.Chunk...)
		if !reply.EOF && len(reply.Chunk) > 0 {
			continue
		}

		if int64(len(out)) != reply.Size {
			return nil, fmt.Errorf("the copy came up short: %d of %d bytes", len(out), reply.Size)
		}
		if reply.Sum != "" && sumOf(out) != reply.Sum {
			return nil, fmt.Errorf("the copy did not survive the trip from node %d", id)
		}
		return out, nil
	}
}

func (a *API) pushDB(id int, blob []byte) error {
	sum := sumOf(blob)
	for offset := 0; offset < len(blob); offset += dbChunk {
		end := offset + dbChunk
		if end > len(blob) {
			end = len(blob)
		}
		last := end == len(blob)

		chunk := map[string]any{
			"offset": offset,
			"chunk":  blob[offset:end],
			"eof":    last,
		}
		if last {
			chunk["sum"] = sum
		}

		if _, err := a.fleet.Ask(id, "db.put", chunk); err != nil {
			return err
		}
	}
	return nil
}

func (a *API) getDB(w http.ResponseWriter, r *http.Request) {
	id, err := a.freshest()
	if err != nil {
		sendFail(w, err)
		return
	}
	blob, err := a.pullDB(id)
	if err != nil {
		sendFail(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="qd-network.db"`)
	w.Write(blob)
}

func (a *API) importDB(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("db")
	if err != nil {
		sendFail(w, err)
		return
	}
	defer file.Close()

	blob, err := io.ReadAll(io.LimitReader(file, 256<<20))
	if err != nil {
		sendFail(w, err)
		return
	}
	results := a.spread(blob)
	a.cache.forget()
	if err := worstOf(results); err != nil {
		sendFailWith(w, err, results)
		return
	}
	sendOK(w, map[string]any{"nodes": results})
}

func worstOf(results []WriteResult) error {
	took, asked := 0, 0
	refused := []string{}
	for _, r := range results {
		if r.Skipped {
			continue
		}
		asked++
		if r.OK {
			took++
			continue
		}
		refused = append(refused, fmt.Sprintf("%s: %s", r.Tag, r.Error))
	}
	if took > 0 || asked == 0 {
		return nil
	}
	return fmt.Errorf("%d of %d nodes refused it — %s",
		len(refused), asked, strings.Join(refused, "; "))
}

func (a *API) syncNetwork(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NodeID int `json:"nodeId"`
	}
	bindBody(r, &body)

	id, err := a.freshest()
	if err != nil {
		sendFail(w, err)
		return
	}
	if body.NodeID == id {
		sendFail(w, fmt.Errorf("that node is the freshest one, there is nothing newer to give it"))
		return
	}

	blob, err := a.pullDB(id)
	if err != nil {
		sendFail(w, err)
		return
	}

	results := []WriteResult{}
	targets := a.fleet.Live()
	if body.NodeID != 0 {
		targets = a.fleet.Nodes()
	}
	for _, n := range targets {
		if n.ID == id {
			continue
		}
		if body.NodeID != 0 && n.ID != body.NodeID {
			continue
		}
		results = append(results, a.push(n, blob))
	}
	a.cache.forget()
	if err := worstOf(results); err != nil {
		sendFailWith(w, err, results)
		return
	}
	a.seedEntrypoints()
	sendOK(w, map[string]any{"source": id, "nodes": results})
}

func (a *API) spread(blob []byte) []WriteResult {
	nodes := a.fleet.Live()
	results := make([]WriteResult, 0, len(nodes))
	for _, n := range nodes {
		results = append(results, a.push(n, blob))
	}
	sort.Slice(results, func(i, j int) bool { return results[i].NodeID < results[j].NodeID })
	return results
}

func (a *API) push(n NodeAddress, blob []byte) WriteResult {
	result := WriteResult{NodeID: n.ID, Tag: n.Tag, OK: true}
	if err := a.pushDB(n.ID, blob); err != nil {
		result.OK = false
		result.Error = err.Error()
	}
	return result
}
