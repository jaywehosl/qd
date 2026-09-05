// Package publish drives a network revision out to the nodes in three phases.
//
// The phases exist so that a half-delivered rollout never becomes a half-applied
// one: plan builds every node's configuration, push ships it to sit beside the
// running revision without taking effect, and apply is the only step that
// changes what a node is doing.
package publish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/jaywehosl/quic-diver/internal/netstate"
)

type State string

const (
	StatePlanned  State = "planned"
	StatePushing  State = "pushing"
	StateStaged   State = "staged"
	StateApplying State = "applying"
	StateApplied  State = "applied"
	StateFailed   State = "failed"
	StateSkipped  State = "skipped"
)

type Phase string

const (
	PhasePlan  Phase = "plan"
	PhasePush  Phase = "push"
	PhaseApply Phase = "apply"
	PhaseDone  Phase = "done"
)

// ErrUnreachable separates "this node is not listening right now" from "this
// node refused what we sent". The first is skipped and picks the revision up on
// reconnect; the second is a failure the operator has to look at.
var ErrUnreachable = errors.New("publish: no control channel")

type Transport interface {
	// Push stores the blob beside the node's running revision. It does not
	// activate anything, and pushing the same revision twice is expected —
	// a broken transfer is retried by sending it again.
	Push(ctx context.Context, nodeID, revision int, blob []byte, sum string) error
	// Apply activates a revision the node has already staged. `sum` is the
	// checksum the node must find on what it holds; a mismatch means the push
	// never completed, and the node keeps running what it was running.
	Apply(ctx context.Context, nodeID, revision int, sum string) error
	// Reachable reports whether a control channel is open right now. Used only
	// to draw the plan: push is what actually decides, because a channel can
	// close between the two.
	Reachable(nodeID int) bool
}

type Options struct {
	// Attempts per node per phase, including the first.
	Attempts int
	Backoff  time.Duration
}

func (o Options) withDefaults() Options {
	if o.Attempts < 1 {
		o.Attempts = 3
	}
	if o.Backoff <= 0 {
		o.Backoff = 3 * time.Second
	}
	return o
}

// NodeRun is one node's progress through the rollout.
type NodeRun struct {
	ID       int           `json:"id"`
	Name     string        `json:"name"`
	Role     netstate.Role `json:"role"`
	Bytes    int           `json:"bytes"`
	Sum      string        `json:"sum"`
	State    State         `json:"state"`
	Attempts int           `json:"attempts,omitempty"`
	Error    string        `json:"error,omitempty"`
	Reason   string        `json:"reason,omitempty"`

	blob []byte
}

type Job struct {
	mu       sync.Mutex
	revision int
	phase    Phase
	nodes    []*NodeRun
	opts     Options
}

// Plan freezes a state into per-node configurations.
//
// Disabled nodes are not targets at all — they are not part of the network, so
// they are neither pushed to nor reported as skipped. Skipped means something
// else entirely: a node that should be receiving this and cannot be reached.
func Plan(s *netstate.State, tr Transport, opts Options) (*Job, error) {
	j := &Job{revision: s.Revision, phase: PhasePlan, opts: opts.withDefaults()}

	for _, n := range s.Nodes {
		if !n.Enable {
			continue
		}
		cfg, err := netstate.Project(n.ID, s)
		if err != nil {
			return nil, err
		}
		blob, err := json.Marshal(cfg)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(blob)

		run := &NodeRun{
			ID:    n.ID,
			Name:  n.Tag,
			Role:  n.Role,
			Bytes: len(blob),
			Sum:   hex.EncodeToString(sum[:]),
			State: StatePlanned,
			blob:  blob,
		}
		if tr != nil && !tr.Reachable(n.ID) {
			run.State = StateSkipped
			run.Reason = "no control channel"
		}
		j.nodes = append(j.nodes, run)
	}
	return j, nil
}

// Push ships the planned configuration. Nodes run concurrently: a rollout across
// twenty nodes should not cost twenty timeouts.
//
// `only` restricts the push to the given node ids, which is what a retry of the
// failures is — the successful nodes are already holding the revision and
// re-sending to them would be work for nothing.
func (j *Job) Push(ctx context.Context, tr Transport, only []int) {
	j.setPhase(PhasePush)
	j.each(only, func(n *NodeRun) bool {
		// Skipped nodes stay skipped until the operator asks for them by name;
		// a blanket push should not spend its retries on a node already known
		// to be absent.
		return n.State == StatePlanned || n.State == StateFailed ||
			(n.State == StateSkipped && len(only) > 0)
	}, func(n *NodeRun) {
		j.attempt(ctx, n, StatePushing, StateStaged, func() error {
			return tr.Push(ctx, n.ID, j.revision, n.blob, n.Sum)
		})
	})
}

// Apply activates the revision on every node that actually staged it.
//
// The filter is the whole point of the three-phase split: a node whose push
// failed is not asked to apply, so an apply can never activate a configuration
// that only half arrived.
func (j *Job) Apply(ctx context.Context, tr Transport) {
	j.setPhase(PhaseApply)
	j.each(nil, func(n *NodeRun) bool { return n.State == StateStaged }, func(n *NodeRun) {
		j.attempt(ctx, n, StateApplying, StateApplied, func() error {
			return tr.Apply(ctx, n.ID, j.revision, n.Sum)
		})
	})
	j.setPhase(PhaseDone)
}

// attempt runs one node's step with the configured retries, moving it through
// `busy` and landing it on `done`, StateFailed or StateSkipped.
func (j *Job) attempt(ctx context.Context, n *NodeRun, busy, done State, do func() error) {
	j.set(n, func() {
		n.State = busy
		n.Attempts = 0
		n.Error = ""
		n.Reason = ""
	})

	var last error
	for i := 0; i < j.opts.Attempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				j.set(n, func() { n.State = StateFailed; n.Error = ctx.Err().Error() })
				return
			case <-time.After(j.opts.Backoff):
			}
		}
		j.set(n, func() { n.Attempts = i + 1 })

		last = do()
		if last == nil {
			j.set(n, func() { n.State = done; n.Error = "" })
			return
		}
		// A closed channel is not a failure to retry into: the node is not
		// there, and it will collect this revision when it comes back.
		if errors.Is(last, ErrUnreachable) {
			j.set(n, func() {
				n.State = StateSkipped
				n.Reason = "no control channel"
			})
			return
		}
	}
	j.set(n, func() { n.State = StateFailed; n.Error = last.Error() })
}

func (j *Job) each(only []int, want func(*NodeRun) bool, do func(*NodeRun)) {
	wanted := map[int]bool{}
	for _, id := range only {
		wanted[id] = true
	}

	var wg sync.WaitGroup
	j.mu.Lock()
	targets := make([]*NodeRun, 0, len(j.nodes))
	for _, n := range j.nodes {
		if len(only) > 0 && !wanted[n.ID] {
			continue
		}
		if want(n) {
			targets = append(targets, n)
		}
	}
	j.mu.Unlock()

	for _, n := range targets {
		wg.Add(1)
		go func(n *NodeRun) {
			defer wg.Done()
			do(n)
		}(n)
	}
	wg.Wait()
}

func (j *Job) set(n *NodeRun, fn func()) {
	j.mu.Lock()
	defer j.mu.Unlock()
	fn()
}

func (j *Job) setPhase(p Phase) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.phase = p
}

func (j *Job) Revision() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.revision
}

// Status is a copy, taken under the lock, so the status endpoint can be polled
// while the rollout is running without reading a node mid-write.
type Status struct {
	Revision int        `json:"revision"`
	Phase    Phase      `json:"phase"`
	Nodes    []*NodeRun `json:"nodes"`
}

func (j *Job) Status() Status {
	j.mu.Lock()
	defer j.mu.Unlock()

	out := make([]*NodeRun, 0, len(j.nodes))
	for _, n := range j.nodes {
		c := *n
		c.blob = nil
		out = append(out, &c)
	}
	return Status{Revision: j.revision, Phase: j.phase, Nodes: out}
}

// FailedIDs is what the dialog's "Retry N failed" button acts on.
func (j *Job) FailedIDs() []int {
	j.mu.Lock()
	defer j.mu.Unlock()

	out := []int{}
	for _, n := range j.nodes {
		if n.State == StateFailed {
			out = append(out, n.ID)
		}
	}
	return out
}
