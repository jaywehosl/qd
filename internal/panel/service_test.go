package panel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jaywehosl/quic-diver/internal/publish"
	"github.com/jaywehosl/quic-diver/internal/store"
)

// The transport is the only fake here: everything else — the database, the
// projection, the rollout machine, the handlers — is the real thing. The point
// of this file is that they compose, which no amount of unit testing each of
// them separately can establish.
type fakeTransport struct {
	mu          sync.Mutex
	unreachable map[int]bool
	pushErr     map[int]error
	staged      map[int]string
	applied     map[int]int
}

func newTransport() *fakeTransport {
	return &fakeTransport{
		unreachable: map[int]bool{},
		pushErr:     map[int]error{},
		staged:      map[int]string{},
		applied:     map[int]int{},
	}
}

func (f *fakeTransport) Reachable(id int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.unreachable[id]
}

func (f *fakeTransport) Push(_ context.Context, id, rev int, _ []byte, sum string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.unreachable[id] {
		return publish.ErrUnreachable
	}
	if err := f.pushErr[id]; err != nil {
		return err
	}
	f.staged[id] = sum
	return nil
}

func (f *fakeTransport) Apply(_ context.Context, id, rev int, sum string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.unreachable[id] {
		return publish.ErrUnreachable
	}
	if f.staged[id] != sum {
		return errors.New("nothing staged with that checksum")
	}
	f.applied[id] = rev
	return nil
}

type rig struct {
	db  *store.DB
	tr  *fakeTransport
	mux *http.ServeMux
	now int64
}

func newRig(t *testing.T) *rig {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	r := &rig{db: db, tr: newTransport(), mux: http.NewServeMux(), now: 1_700_000_000_000}
	svc := New(Config{
		DB:        db,
		Transport: r.tr,
		Options:   publish.Options{Attempts: 2, Backoff: time.Millisecond},
		Now:       func() int64 { r.now += 1000; return r.now },
	})
	svc.Routes(r.mux)
	return r
}

func (r *rig) seed(t *testing.T) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := r.db.SQL().Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO nodes (id, tag, address, port, role, enable, created_at)
	      VALUES (1, 'node-1', '1.2.3.4', 443, 'ingress', 1, 1)`)
	exec(`INSERT INTO nodes (id, tag, address, port, role, enable, created_at)
	      VALUES (2, 'node-4', '5.6.7.8', 443, 'egress', 1, 1)`)
	exec(`INSERT INTO entrypoints (id, node_id, port, enable, created_at) VALUES (10, 1, 443, 1, 1)`)
	exec(`INSERT INTO entrypoints (id, node_id, port, enable, created_at) VALUES (20, 2, 443, 1, 1)`)
	exec(`INSERT INTO groups (id, tag, allow_exit, created_at) VALUES (100, 'russia-in', 1, 1)`)
	exec(`INSERT INTO group_entrypoints (group_id, entrypoint_id) VALUES (100, 10)`)
	exec(`INSERT INTO clients (id, tag, uuid, group_id, enable, created_at)
	      VALUES (1000, 'vasya', 'u-vasya', 100, 1, 1)`)
	exec(`INSERT INTO network (id, key, created_at) VALUES (1, 'network-key', 1)`)
	if _, err := r.db.Touch(1); err != nil {
		t.Fatal(err)
	}
}

type envelope struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}

func (r *rig) call(t *testing.T, method, path, body string) envelope {
	t.Helper()

	var rdr *strings.Reader
	if body == "" {
		rdr = strings.NewReader("")
	} else {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.mux.ServeHTTP(w, req)

	var env envelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("%s %s: unreadable reply %q", method, path, w.Body.String())
	}
	return env
}

func into(t *testing.T, raw json.RawMessage, dst any) {
	t.Helper()
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("decoding %s: %v", raw, err)
	}
}

// The whole flow through real handlers and a real database.
func TestPublishVerticalSlice(t *testing.T) {
	r := newRig(t)
	r.seed(t)

	// The draft summary names what changed, in the operator's words.
	var draft struct {
		Revision          int `json:"revision"`
		PublishedRevision int `json:"publishedRevision"`
		Changes           []struct {
			Kind, Name, Action string
		} `json:"changes"`
	}
	env := r.call(t, "GET", "/panel/api/publish/draft", "")
	if !env.Success {
		t.Fatal(env.Msg)
	}
	into(t, env.Obj, &draft)
	if draft.Revision != 1 || draft.PublishedRevision != 0 {
		t.Fatalf("draft %d against published %d", draft.Revision, draft.PublishedRevision)
	}
	if len(draft.Changes) == 0 {
		t.Fatal("a first draft reported nothing to publish")
	}

	// Plan freezes it and builds each node's configuration.
	var plan struct {
		Revision int `json:"revision"`
		Targets  []struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			Role  string `json:"role"`
			Bytes int    `json:"bytes"`
		} `json:"targets"`
		Skipped []struct{ ID int } `json:"skipped"`
	}
	env = r.call(t, "POST", "/panel/api/publish/plan", "")
	if !env.Success {
		t.Fatal(env.Msg)
	}
	into(t, env.Obj, &plan)
	if plan.Revision != 1 || len(plan.Targets) != 2 || len(plan.Skipped) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
	for _, tg := range plan.Targets {
		if tg.Bytes == 0 {
			t.Fatalf("node %s planned with an empty configuration", tg.Name)
		}
	}

	env = r.call(t, "POST", "/panel/api/publish/push", `{"revision":1}`)
	if !env.Success {
		t.Fatal(env.Msg)
	}
	env = r.call(t, "POST", "/panel/api/publish/apply", `{"revision":1}`)
	if !env.Success {
		t.Fatal(env.Msg)
	}

	r.tr.mu.Lock()
	applied := len(r.tr.applied)
	r.tr.mu.Unlock()
	if applied != 2 {
		t.Fatalf("%d nodes ended up applied", applied)
	}

	// And once it is out, the draft is empty again — both header buttons go.
	env = r.call(t, "GET", "/panel/api/publish/draft", "")
	into(t, env.Obj, &draft)
	if len(draft.Changes) != 0 {
		t.Fatalf("a published draft still reports %+v", draft.Changes)
	}
	if draft.PublishedRevision != 1 {
		t.Fatalf("publishedRevision = %d", draft.PublishedRevision)
	}
}

// The summary has to describe the draft against what the NODES hold, not
// against an empty database — otherwise every publish reads as "everything
// changed" and the operator stops reading it.
func TestDraftIsMeasuredAgainstWhatWasPublished(t *testing.T) {
	r := newRig(t)
	r.seed(t)
	r.call(t, "POST", "/panel/api/publish/plan", "")
	r.call(t, "POST", "/panel/api/publish/push", `{"revision":1}`)
	r.call(t, "POST", "/panel/api/publish/apply", `{"revision":1}`)

	if _, err := r.db.Touch(r.now); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.SQL().Exec(`UPDATE clients SET comment = 'paid' WHERE id = 1000`); err != nil {
		t.Fatal(err)
	}

	var draft struct {
		Revision          int `json:"revision"`
		PublishedRevision int `json:"publishedRevision"`
		Changes           []struct{ Kind, Name, Action string }
	}
	env := r.call(t, "GET", "/panel/api/publish/draft", "")
	into(t, env.Obj, &draft)

	if draft.PublishedRevision != 1 || draft.Revision != 2 {
		t.Fatalf("revision %d against published %d", draft.Revision, draft.PublishedRevision)
	}
	if len(draft.Changes) != 1 {
		t.Fatalf("want one change, got %+v", draft.Changes)
	}
	c := draft.Changes[0]
	if c.Kind != "client" || c.Name != "vasya" || c.Action != "updated" {
		t.Fatalf("change = %+v", c)
	}
}

// The header hides both buttons on an empty list, so `changes` must always be a
// list — a nil slice would arrive as JSON null and crash the reader.
func TestChangesIsNeverNull(t *testing.T) {
	r := newRig(t)
	env := r.call(t, "GET", "/panel/api/publish/draft", "")
	if !strings.Contains(string(env.Obj), `"changes":[]`) {
		t.Fatalf("empty draft encoded as %s", env.Obj)
	}
}

// A discard is local. Nothing may reach a node, and the record of what nodes
// are running must not move.
func TestDiscardRewindsTheDraftAndTellsNobody(t *testing.T) {
	r := newRig(t)
	r.seed(t)
	r.call(t, "POST", "/panel/api/publish/plan", "")
	r.call(t, "POST", "/panel/api/publish/push", `{"revision":1}`)
	r.call(t, "POST", "/panel/api/publish/apply", `{"revision":1}`)

	if _, err := r.db.Touch(r.now); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.SQL().Exec(`DELETE FROM clients WHERE id = 1000`); err != nil {
		t.Fatal(err)
	}

	r.tr.mu.Lock()
	pushesBefore := len(r.tr.staged)
	r.tr.mu.Unlock()

	env := r.call(t, "POST", "/panel/api/publish/discard", "")
	if !env.Success {
		t.Fatal(env.Msg)
	}

	state, err := r.db.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Clients) != 1 {
		t.Fatalf("discard did not restore the client: %+v", state.Clients)
	}

	r.tr.mu.Lock()
	pushesAfter := len(r.tr.staged)
	r.tr.mu.Unlock()
	if pushesAfter != pushesBefore {
		t.Fatal("a discard sent something to a node")
	}

	progress, err := r.db.NodeProgress()
	if err != nil {
		t.Fatal(err)
	}
	if progress[1].Applied != 1 {
		t.Fatalf("a discard rewound what node 1 is running: %+v", progress[1])
	}
}

// What each node is actually running is recorded, so the nodes list can show
// which ones still owe an apply.
func TestProgressIsRecordedPerNode(t *testing.T) {
	r := newRig(t)
	r.seed(t)
	r.tr.pushErr[2] = errors.New("control channel reset")

	r.call(t, "POST", "/panel/api/publish/plan", "")
	r.call(t, "POST", "/panel/api/publish/push", `{"revision":1}`)
	r.call(t, "POST", "/panel/api/publish/apply", `{"revision":1}`)

	progress, err := r.db.NodeProgress()
	if err != nil {
		t.Fatal(err)
	}
	if progress[1].Applied != 1 {
		t.Fatalf("node 1 progress = %+v", progress[1])
	}
	if progress[2].Applied != 0 {
		t.Fatalf("node 2 failed its push but is recorded at revision %d", progress[2].Applied)
	}
}

// A node that was left behind must keep its recorded revision when a later
// rollout succeeds elsewhere — the record only ever moves forward.
func TestProgressNeverGoesBackwards(t *testing.T) {
	r := newRig(t)
	r.seed(t)
	r.call(t, "POST", "/panel/api/publish/plan", "")
	r.call(t, "POST", "/panel/api/publish/push", `{"revision":1}`)
	r.call(t, "POST", "/panel/api/publish/apply", `{"revision":1}`)

	if err := r.db.RecordNodeProgress(1, 0, 0, "online", r.now); err != nil {
		t.Fatal(err)
	}
	progress, err := r.db.NodeProgress()
	if err != nil {
		t.Fatal(err)
	}
	if progress[1].Applied != 1 {
		t.Fatalf("a stale report dragged node 1 back to %d", progress[1].Applied)
	}
}

func TestUnreachableNodeIsPlannedAsSkipped(t *testing.T) {
	r := newRig(t)
	r.seed(t)
	r.tr.unreachable[2] = true

	var plan struct {
		Targets []struct{ ID int }
		Skipped []struct {
			ID     int    `json:"id"`
			Name   string `json:"name"`
			Reason string `json:"reason"`
		}
	}
	env := r.call(t, "POST", "/panel/api/publish/plan", "")
	into(t, env.Obj, &plan)

	if len(plan.Targets) != 1 || plan.Targets[0].ID != 1 {
		t.Fatalf("targets = %+v", plan.Targets)
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].ID != 2 || plan.Skipped[0].Reason == "" {
		t.Fatalf("skipped = %+v", plan.Skipped)
	}
}

// The dialog can be closed and reopened; reopening resumes the run rather than
// starting a second one.
func TestStatusResumesTheRunInFlight(t *testing.T) {
	r := newRig(t)
	r.seed(t)

	env := r.call(t, "GET", "/panel/api/publish/status", "")
	if string(env.Obj) != "null" {
		t.Fatalf("status before any plan = %s", env.Obj)
	}

	r.call(t, "POST", "/panel/api/publish/plan", "")
	r.call(t, "POST", "/panel/api/publish/push", `{"revision":1}`)

	var st struct {
		Revision int    `json:"revision"`
		Phase    string `json:"phase"`
		Nodes    []struct {
			ID    int    `json:"id"`
			State string `json:"state"`
		} `json:"nodes"`
	}
	env = r.call(t, "GET", "/panel/api/publish/status", "")
	into(t, env.Obj, &st)

	if st.Revision != 1 || st.Phase != "push" {
		t.Fatalf("status = revision %d phase %s", st.Revision, st.Phase)
	}
	for _, n := range st.Nodes {
		if n.State != "staged" {
			t.Fatalf("node %d is %s", n.ID, n.State)
		}
	}
}

// A dialog reopened after a newer plan must not push one revision and apply
// another.
func TestARevisionThatIsNotInFlightIsRefused(t *testing.T) {
	r := newRig(t)
	r.seed(t)
	r.call(t, "POST", "/panel/api/publish/plan", "")

	env := r.call(t, "POST", "/panel/api/publish/push", `{"revision":99}`)
	if env.Success {
		t.Fatal("pushed a revision that is not being rolled out")
	}
	if !strings.Contains(env.Msg, "not the one being rolled out") {
		t.Fatalf("refused for the wrong reason: %q", env.Msg)
	}
}

func TestPushBeforePlanIsRefused(t *testing.T) {
	r := newRig(t)
	r.seed(t)

	env := r.call(t, "POST", "/panel/api/publish/push", `{"revision":1}`)
	if env.Success {
		t.Fatal("pushed with no plan")
	}
	if !strings.Contains(env.Msg, "plan first") {
		t.Fatalf("refused for the wrong reason: %q", env.Msg)
	}
}

func TestPlanWithNothingToPublishIsRefused(t *testing.T) {
	r := newRig(t)
	r.seed(t)
	r.call(t, "POST", "/panel/api/publish/plan", "")

	env := r.call(t, "POST", "/panel/api/publish/plan", "")
	if env.Success {
		t.Fatal("planned a second time with no draft open")
	}
	if !strings.Contains(env.Msg, "nothing to publish") {
		t.Fatalf("refused for the wrong reason: %q", env.Msg)
	}
}

// Retrying is what the dialog's "Retry N failed" does, and it must land.
func TestRetryAfterAFailedPush(t *testing.T) {
	r := newRig(t)
	r.seed(t)
	r.tr.pushErr[2] = errors.New("reset")

	r.call(t, "POST", "/panel/api/publish/plan", "")
	r.call(t, "POST", "/panel/api/publish/push", `{"revision":1}`)

	var rows []struct {
		ID    int    `json:"id"`
		State string `json:"state"`
		Error string `json:"error"`
	}
	env := r.call(t, "POST", "/panel/api/publish/apply", `{"revision":1}`)
	into(t, env.Obj, &rows)
	for _, row := range rows {
		if row.ID == 2 && row.State != "failed" {
			t.Fatalf("node 2 is %s after a failed push", row.State)
		}
	}

	r.tr.mu.Lock()
	delete(r.tr.pushErr, 2)
	r.tr.mu.Unlock()

	r.call(t, "POST", "/panel/api/publish/push", `{"revision":1,"nodeIds":[2]}`)
	env = r.call(t, "POST", "/panel/api/publish/apply", `{"revision":1}`)
	into(t, env.Obj, &rows)

	for _, row := range rows {
		if row.State != "applied" {
			t.Fatalf("node %d is %s after the retry (%s)", row.ID, row.State, row.Error)
		}
	}
	progress, err := r.db.NodeProgress()
	if err != nil {
		t.Fatal(err)
	}
	if progress[2].Applied != 1 {
		t.Fatalf("the retried node is recorded at %d", progress[2].Applied)
	}
}

// The revision must be recorded before any of it leaves the machine: a panel
// that dies mid-rollout must not leave nodes running something it has no record
// of.
func TestRevisionIsRecordedBeforeAnythingIsSent(t *testing.T) {
	r := newRig(t)
	r.seed(t)

	r.call(t, "POST", "/panel/api/publish/plan", "")

	pub, err := r.db.LastPublished()
	if err != nil {
		t.Fatal(err)
	}
	if pub == nil || pub.Number != 1 {
		t.Fatalf("plan did not record the revision: %+v", pub)
	}

	// And the snapshot is readable, which is what a discard and a rollback need.
	state, err := r.db.RevisionState(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Clients) != 1 || state.Clients[0].Tag != "vasya" {
		t.Fatalf("snapshot = %+v", state.Clients)
	}
}
