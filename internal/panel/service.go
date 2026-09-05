// Package panel is the panel's own HTTP service: the place where the database,
// the projection, the rollout machine and the control channel are finally one
// program rather than five that fit together on paper.
package panel

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/jaywehosl/quic-diver/internal/netstate"
	"github.com/jaywehosl/quic-diver/internal/publish"
	"github.com/jaywehosl/quic-diver/internal/store"
)

type Service struct {
	db        *store.DB
	transport publish.Transport
	opts      publish.Options
	now       func() int64

	// One rollout at a time. The dialog can be closed and reopened, which is
	// what `job` outliving a request is for; two rollouts at once would have
	// nodes racing each other to a revision.
	mu  sync.Mutex
	job *publish.Job
}

type Config struct {
	DB        *store.DB
	Transport publish.Transport
	Options   publish.Options
	// Now is injectable so tests are not at the mercy of the clock.
	Now func() int64
}

func New(cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = func() int64 { return time.Now().UnixMilli() }
	}
	return &Service{db: cfg.DB, transport: cfg.Transport, opts: cfg.Options, now: now}
}

func (s *Service) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/panel/api/publish/draft", s.handleDraft)
	mux.HandleFunc("/panel/api/publish/discard", s.handleDiscard)
	mux.HandleFunc("/panel/api/publish/plan", s.handlePlan)
	mux.HandleFunc("/panel/api/publish/push", s.handlePush)
	mux.HandleFunc("/panel/api/publish/apply", s.handleApply)
	mux.HandleFunc("/panel/api/publish/status", s.handleStatus)
}

// publishedState reads back the snapshot the nodes are running, which is what a
// draft is measured against.
func (s *Service) publishedState() (*netstate.State, int, error) {
	pub, err := s.db.LastPublished()
	if err != nil || pub == nil {
		return nil, 0, err
	}
	st, err := s.db.RevisionState(pub.Number)
	if err != nil {
		return nil, 0, err
	}
	return st, pub.Number, nil
}

func (s *Service) handleDraft(w http.ResponseWriter, r *http.Request) {
	draft, err := s.db.Draft()
	if err != nil {
		fail(w, err)
		return
	}
	published, publishedRev, err := s.publishedState()
	if err != nil {
		fail(w, err)
		return
	}
	if draft == nil {
		// No draft means the tables match what the nodes hold. The header shows
		// neither button, so an empty change list is the honest answer rather
		// than an absent object.
		ok(w, map[string]any{
			"revision":          publishedRev,
			"publishedRevision": publishedRev,
			"changes":           []netstate.Change{},
		})
		return
	}

	current, err := s.db.LoadState()
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{
		"revision":          draft.Number,
		"publishedRevision": publishedRev,
		"changes":           netstate.Diff(published, current),
	})
}

func (s *Service) handleDiscard(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Discard(); err != nil {
		fail(w, err)
		return
	}
	_, rev, err := s.publishedState()
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"revision": rev})
}

// handlePlan freezes the draft into a revision and builds every node's
// configuration from it.
//
// Publishing happens here rather than after a successful apply on purpose: the
// revision has to exist and be recorded before any of it leaves the machine, or
// a panel that dies mid-rollout would have nodes running a revision it has no
// record of.
func (s *Service) handlePlan(w http.ResponseWriter, r *http.Request) {
	state, err := s.db.Publish(s.now())
	if errors.Is(err, store.ErrNothingToPublish) {
		fail(w, errors.New("nothing to publish"))
		return
	}
	if err != nil {
		fail(w, err)
		return
	}

	job, err := publish.Plan(state, s.transport, s.opts)
	if err != nil {
		fail(w, err)
		return
	}

	s.mu.Lock()
	s.job = job
	s.mu.Unlock()

	targets := []map[string]any{}
	skipped := []map[string]any{}
	for _, n := range job.Status().Nodes {
		if n.State == publish.StateSkipped {
			skipped = append(skipped, map[string]any{"id": n.ID, "name": n.Name, "reason": n.Reason})
			continue
		}
		targets = append(targets, map[string]any{
			"id": n.ID, "name": n.Name, "role": n.Role, "bytes": n.Bytes,
		})
	}
	ok(w, map[string]any{"revision": state.Revision, "targets": targets, "skipped": skipped})
}

func (s *Service) handlePush(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Revision int   `json:"revision"`
		NodeIDs  []int `json:"nodeIds"`
	}
	if !decode(w, r, &body) {
		return
	}
	job, err := s.jobFor(body.Revision)
	if err != nil {
		fail(w, err)
		return
	}

	job.Push(r.Context(), s.transport, body.NodeIDs)
	s.recordProgress(job)
	ok(w, nodeStates(job))
}

func (s *Service) handleApply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Revision int `json:"revision"`
	}
	if !decode(w, r, &body) {
		return
	}
	job, err := s.jobFor(body.Revision)
	if err != nil {
		fail(w, err)
		return
	}

	job.Apply(r.Context(), s.transport)
	s.recordProgress(job)
	ok(w, nodeStates(job))
}

func (s *Service) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	job := s.job
	s.mu.Unlock()

	if job == nil {
		ok(w, nil)
		return
	}
	st := job.Status()
	ok(w, map[string]any{
		"revision": st.Revision,
		"phase":    st.Phase,
		"nodes":    nodeStates(job),
	})
}

// jobFor refuses to act on a revision other than the one in flight. Without the
// check a stale dialog — reopened after a later plan — would push one revision
// and apply another.
func (s *Service) jobFor(revision int) (*publish.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.job == nil {
		return nil, errors.New("no rollout is in flight — plan first")
	}
	if revision != 0 && revision != s.job.Revision() {
		return nil, errors.New("that revision is not the one being rolled out")
	}
	return s.job, nil
}

// recordProgress writes what each node is actually running back to the
// telemetry half. It is what the nodes list compares against to show drift, and
// it survives a discard because it is a fact rather than a draft.
func (s *Service) recordProgress(job *publish.Job) {
	st := job.Status()
	for _, n := range st.Nodes {
		var applied, staged int
		switch n.State {
		case publish.StateApplied:
			applied = st.Revision
			staged = st.Revision
		case publish.StateStaged, publish.StateApplying:
			staged = st.Revision
		default:
			continue
		}
		if err := s.db.RecordNodeProgress(n.ID, applied, staged, string(n.State), s.now()); err != nil {
			// Losing a progress note must not fail a rollout that worked: the
			// node is running the revision either way, and the next report
			// puts the record straight.
			continue
		}
	}
}

func nodeStates(job *publish.Job) []map[string]any {
	out := []map[string]any{}
	for _, n := range job.Status().Nodes {
		row := map[string]any{"id": n.ID, "name": n.Name, "state": n.State}
		if n.Attempts > 0 {
			row["attempts"] = n.Attempts
		}
		if n.Error != "" {
			row["error"] = n.Error
		}
		if n.Reason != "" {
			row["reason"] = n.Reason
		}
		if n.State == publish.StateApplied {
			row["appliedRevision"] = job.Revision()
		}
		out = append(out, row)
	}
	return out
}

// The envelope every endpoint in this system answers with.
func ok(w http.ResponseWriter, obj any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true, "msg": "", "obj": obj})
}

func fail(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": false, "msg": err.Error(), "obj": nil})
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Method != http.MethodPost {
		fail(w, errors.New("method not allowed"))
		return false
	}
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		fail(w, err)
		return false
	}
	return true
}
