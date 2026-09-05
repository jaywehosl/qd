package publish

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jaywehosl/quic-diver/internal/netstate"
)

// fakeTransport records everything it is asked to do and can be told to fail a
// given node — which is the only way to test a torn rollout without tearing a
// real network.
type fakeTransport struct {
	mu sync.Mutex

	unreachable map[int]bool
	pushErr     map[int]error
	applyErr    map[int]error
	// Fail the first N attempts for a node, then succeed. Models a transfer
	// that breaks and is retried.
	pushFlaky map[int]int

	pushed  []call
	applied []call
	staged  map[int]string
}

type call struct {
	NodeID   int
	Revision int
	Sum      string
	Bytes    int
}

func newFake() *fakeTransport {
	return &fakeTransport{
		unreachable: map[int]bool{},
		pushErr:     map[int]error{},
		applyErr:    map[int]error{},
		pushFlaky:   map[int]int{},
		staged:      map[int]string{},
	}
}

func (f *fakeTransport) Reachable(id int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.unreachable[id]
}

func (f *fakeTransport) Push(_ context.Context, id, rev int, blob []byte, sum string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pushed = append(f.pushed, call{id, rev, sum, len(blob)})

	if f.unreachable[id] {
		return ErrUnreachable
	}
	if n := f.pushFlaky[id]; n > 0 {
		f.pushFlaky[id] = n - 1
		return errors.New("transfer reset")
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
	f.applied = append(f.applied, call{id, rev, sum, 0})

	if f.unreachable[id] {
		return ErrUnreachable
	}
	if err := f.applyErr[id]; err != nil {
		return err
	}
	// A node activates only what it actually holds. Asking it to apply a
	// revision it never staged is the failure the three phases exist to make
	// impossible, so the fake refuses it loudly rather than pretending.
	if f.staged[id] != sum {
		return errors.New("nothing staged with that checksum")
	}
	return nil
}

func (f *fakeTransport) pushCount(id int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.pushed {
		if c.NodeID == id {
			n++
		}
	}
	return n
}

func (f *fakeTransport) appliedIDs() map[int]bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[int]bool{}
	for _, c := range f.applied {
		out[c.NodeID] = true
	}
	return out
}

func fixture() *netstate.State {
	return &netstate.State{
		Revision: 43,
		Nodes: []netstate.Node{
			{ID: 1, Tag: "node-1", Address: "1.2.3.4", Port: 443, Role: netstate.RoleIngress, Enable: true},
			{ID: 2, Tag: "node-4", Address: "5.6.7.8", Port: 443, Role: netstate.RoleEgress, Enable: true},
			{ID: 3, Tag: "node-7", Address: "9.9.9.9", Port: 443, Role: netstate.RoleIngress, Enable: true},
			{ID: 4, Tag: "retired", Address: "0.0.0.0", Port: 443, Role: netstate.RoleEgress, Enable: false},
		},
		Entrypoints: []netstate.Entrypoint{
			{ID: 10, NodeID: 1, Port: 443, Enable: true},
			{ID: 20, NodeID: 2, Port: 443, Enable: true},
			{ID: 30, NodeID: 3, Port: 443, Enable: true},
		},
		Groups: []netstate.Group{
			{ID: 100, Tag: "russia-in", AllowExit: true, EntrypointIDs: []int{10, 30}},
		},
		Clients: []netstate.Client{
			{ID: 1000, Tag: "vasya", UUID: "uuid-vasya", GroupID: 100, Enable: true},
		},
	}
}

// Tests run with no backoff; the retry COUNT is what matters and waiting three
// seconds nine times to observe it would be a slow way to learn nothing.
func fast() Options { return Options{Attempts: 3, Backoff: 1} }

func mustPlan(t *testing.T, s *netstate.State, tr Transport) *Job {
	t.Helper()
	j, err := Plan(s, tr, fast())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	return j
}

func stateOf(st Status, id int) *NodeRun {
	for _, n := range st.Nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func TestHappyPathStagesThenApplies(t *testing.T) {
	tr := newFake()
	j := mustPlan(t, fixture(), tr)

	st := j.Status()
	if len(st.Nodes) != 3 {
		t.Fatalf("planned %d nodes, want 3 (the disabled one is not a target)", len(st.Nodes))
	}
	for _, n := range st.Nodes {
		if n.State != StatePlanned {
			t.Fatalf("node %d planned as %s", n.ID, n.State)
		}
		if n.Bytes == 0 || n.Sum == "" {
			t.Fatalf("node %d planned without a body or checksum", n.ID)
		}
	}

	ctx := context.Background()
	j.Push(ctx, tr, nil)
	for _, n := range j.Status().Nodes {
		if n.State != StateStaged {
			t.Fatalf("node %d is %s after push, want staged", n.ID, n.State)
		}
	}

	j.Apply(ctx, tr)
	st = j.Status()
	if st.Phase != PhaseDone {
		t.Fatalf("phase = %s", st.Phase)
	}
	for _, n := range st.Nodes {
		if n.State != StateApplied {
			t.Fatalf("node %d is %s after apply", n.ID, n.State)
		}
	}
}

// A disabled node is not part of the network. It is not a target and it is not
// "skipped" either — skipped means a node that should have got this and could
// not be reached.
func TestDisabledNodeIsNotATarget(t *testing.T) {
	tr := newFake()
	j := mustPlan(t, fixture(), tr)
	if stateOf(j.Status(), 4) != nil {
		t.Fatal("a disabled node was planned")
	}
	j.Push(context.Background(), tr, nil)
	if tr.pushCount(4) != 0 {
		t.Fatal("a disabled node was pushed to")
	}
}

// The invariant the whole three-phase split exists for.
func TestApplyNeverTouchesANodeThatDidNotStage(t *testing.T) {
	tr := newFake()
	tr.pushErr[2] = errors.New("control channel reset")
	tr.unreachable[3] = true

	j := mustPlan(t, fixture(), tr)
	ctx := context.Background()
	j.Push(ctx, tr, nil)
	j.Apply(ctx, tr)

	got := tr.appliedIDs()
	if !got[1] {
		t.Fatal("the node that staged cleanly was not applied")
	}
	if got[2] {
		t.Fatal("a node whose push failed was told to apply")
	}
	if got[3] {
		t.Fatal("an unreachable node was told to apply")
	}
}

func TestUnreachableIsSkippedNotFailed(t *testing.T) {
	tr := newFake()
	tr.unreachable[3] = true

	j := mustPlan(t, fixture(), tr)
	if n := stateOf(j.Status(), 3); n.State != StateSkipped || n.Reason == "" {
		t.Fatalf("plan marked the unreachable node %s (%q)", n.State, n.Reason)
	}

	j.Push(context.Background(), tr, nil)
	n := stateOf(j.Status(), 3)
	if n.State != StateSkipped {
		t.Fatalf("node 3 is %s after push, want skipped", n.State)
	}
	if len(j.FailedIDs()) != 0 {
		t.Fatalf("a skipped node was counted as failed: %v", j.FailedIDs())
	}
}

// A channel that closes between plan and push has to be caught by the push, not
// by the picture drawn a moment earlier.
func TestChannelLostAfterPlanIsStillSkipped(t *testing.T) {
	tr := newFake()
	j := mustPlan(t, fixture(), tr)
	if n := stateOf(j.Status(), 2); n.State != StatePlanned {
		t.Fatalf("node 2 planned as %s", n.State)
	}

	tr.unreachable[2] = true
	j.Push(context.Background(), tr, nil)

	if n := stateOf(j.Status(), 2); n.State != StateSkipped {
		t.Fatalf("node 2 is %s, want skipped", n.State)
	}
	// Skipping must not burn the retries: the node is not there.
	if got := tr.pushCount(2); got != 1 {
		t.Fatalf("an absent node was retried %d times", got)
	}
}

func TestFailedPushIsRetriedThenReported(t *testing.T) {
	tr := newFake()
	tr.pushErr[2] = errors.New("control channel reset")

	j := mustPlan(t, fixture(), tr)
	j.Push(context.Background(), tr, nil)

	n := stateOf(j.Status(), 2)
	if n.State != StateFailed {
		t.Fatalf("node 2 is %s, want failed", n.State)
	}
	if n.Attempts != 3 {
		t.Fatalf("node 2 was tried %d times, want 3", n.Attempts)
	}
	if n.Error == "" {
		t.Fatal("a failure with no reason attached")
	}
	if got := tr.pushCount(2); got != 3 {
		t.Fatalf("transport saw %d pushes, want 3", got)
	}
}

// A transfer that breaks and then succeeds is the ordinary case the doc
// describes: the incomplete file simply never becomes complete, and sending it
// again finishes the job.
func TestFlakyPushSucceedsOnRetry(t *testing.T) {
	tr := newFake()
	tr.pushFlaky[2] = 2

	j := mustPlan(t, fixture(), tr)
	ctx := context.Background()
	j.Push(ctx, tr, nil)

	n := stateOf(j.Status(), 2)
	if n.State != StateStaged {
		t.Fatalf("node 2 is %s, want staged", n.State)
	}
	if n.Attempts != 3 {
		t.Fatalf("node 2 took %d attempts, want 3", n.Attempts)
	}
	if n.Error != "" {
		t.Fatalf("a recovered node kept its error: %q", n.Error)
	}

	j.Apply(ctx, tr)
	if stateOf(j.Status(), 2).State != StateApplied {
		t.Fatal("a node that recovered on retry was not applied")
	}
}

// "Retry N failed" must not re-ship to nodes already holding the revision.
func TestRetryTouchesOnlyTheFailures(t *testing.T) {
	tr := newFake()
	tr.pushErr[2] = errors.New("reset")

	j := mustPlan(t, fixture(), tr)
	ctx := context.Background()
	j.Push(ctx, tr, nil)

	failed := j.FailedIDs()
	if len(failed) != 1 || failed[0] != 2 {
		t.Fatalf("FailedIDs = %v, want [2]", failed)
	}

	before1, before3 := tr.pushCount(1), tr.pushCount(3)
	delete(tr.pushErr, 2)
	j.Push(ctx, tr, failed)

	if tr.pushCount(1) != before1 || tr.pushCount(3) != before3 {
		t.Fatal("a retry re-shipped to nodes that were already staged")
	}
	if stateOf(j.Status(), 2).State != StateStaged {
		t.Fatal("the retried node did not recover")
	}
	if len(j.FailedIDs()) != 0 {
		t.Fatalf("still failed after a successful retry: %v", j.FailedIDs())
	}
}

// A blanket push must not spend retries on nodes known to be absent, but naming
// one explicitly is the operator saying "try it anyway".
func TestSkippedNodeIsRetriedOnlyWhenNamed(t *testing.T) {
	tr := newFake()
	tr.unreachable[3] = true

	j := mustPlan(t, fixture(), tr)
	ctx := context.Background()
	j.Push(ctx, tr, nil)
	if got := tr.pushCount(3); got != 0 {
		t.Fatalf("a blanket push spent %d attempts on an absent node", got)
	}

	delete(tr.unreachable, 3)
	j.Push(ctx, tr, []int{3})
	if stateOf(j.Status(), 3).State != StateStaged {
		t.Fatal("naming the skipped node did not push to it")
	}
}

func TestApplyFailureLeavesTheNodeFailedNotApplied(t *testing.T) {
	tr := newFake()
	tr.applyErr[1] = errors.New("could not swap the revision")

	j := mustPlan(t, fixture(), tr)
	ctx := context.Background()
	j.Push(ctx, tr, nil)
	j.Apply(ctx, tr)

	if n := stateOf(j.Status(), 1); n.State != StateFailed {
		t.Fatalf("node 1 is %s after a failed apply", n.State)
	}
	for _, id := range []int{2, 3} {
		if stateOf(j.Status(), id).State != StateApplied {
			t.Fatalf("one node's apply failure held node %d up", id)
		}
	}
}

// The checksum that travels with apply must be the one taken of the exact bytes
// pushed, or the node's guard against a partial transfer guards nothing.
func TestApplyCarriesTheChecksumOfWhatWasPushed(t *testing.T) {
	tr := newFake()
	j := mustPlan(t, fixture(), tr)
	ctx := context.Background()
	j.Push(ctx, tr, nil)
	j.Apply(ctx, tr)

	tr.mu.Lock()
	defer tr.mu.Unlock()
	pushSum := map[int]string{}
	for _, c := range tr.pushed {
		pushSum[c.NodeID] = c.Sum
	}
	for _, c := range tr.applied {
		if pushSum[c.NodeID] != c.Sum {
			t.Fatalf("node %d was pushed %s but told to apply %s", c.NodeID, pushSum[c.NodeID], c.Sum)
		}
		if c.Revision != 43 {
			t.Fatalf("node %d was told to apply revision %d", c.NodeID, c.Revision)
		}
	}
}

// Every node gets a different configuration, so a checksum shared between two of
// them would mean the projection collapsed them into one.
func TestEachNodeGetsItsOwnChecksum(t *testing.T) {
	j := mustPlan(t, fixture(), newFake())
	seen := map[string]int{}
	for _, n := range j.Status().Nodes {
		if prev, dup := seen[n.Sum]; dup {
			t.Fatalf("nodes %d and %d were planned with the same checksum", prev, n.ID)
		}
		seen[n.Sum] = n.ID
	}
}

// Status is polled while the rollout runs; it must never hand out the live
// structs or the caller can read a node mid-write.
func TestStatusIsASnapshot(t *testing.T) {
	tr := newFake()
	j := mustPlan(t, fixture(), tr)

	before := j.Status()
	j.Push(context.Background(), tr, nil)

	if before.Phase != PhasePlan {
		t.Fatalf("an earlier snapshot changed phase to %s", before.Phase)
	}
	for _, n := range before.Nodes {
		if n.State != StatePlanned {
			t.Fatalf("an earlier snapshot moved node %d to %s", n.ID, n.State)
		}
		if n.blob != nil {
			t.Fatalf("node %d's body leaked into a status snapshot", n.ID)
		}
	}
}

// The status endpoint is polled while the rollout is in flight, which is the
// one moment the job is being written to from several goroutines at once. Under
// -race this is what proves the lock covers the whole of it.
func TestStatusCanBePolledDuringARollout(t *testing.T) {
	tr := newFake()
	tr.pushFlaky[1] = 1
	tr.pushErr[2] = errors.New("reset")

	j, err := Plan(fixture(), tr, Options{Attempts: 3, Backoff: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var polls sync.WaitGroup
	for i := 0; i < 4; i++ {
		polls.Add(1)
		go func() {
			defer polls.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				st := j.Status()
				_ = st.Phase
				for _, n := range st.Nodes {
					_ = n.State
					_ = n.Attempts
					_ = n.Error
				}
				_ = j.FailedIDs()
			}
		}()
	}

	ctx := context.Background()
	j.Push(ctx, tr, nil)
	j.Apply(ctx, tr)
	close(stop)
	polls.Wait()

	if stateOf(j.Status(), 1).State != StateApplied {
		t.Fatal("polling changed the outcome")
	}
}

func TestPhaseTracksTheRollout(t *testing.T) {
	tr := newFake()
	j := mustPlan(t, fixture(), tr)
	if j.Status().Phase != PhasePlan {
		t.Fatal("a fresh job is not in the plan phase")
	}
	ctx := context.Background()
	j.Push(ctx, tr, nil)
	if j.Status().Phase != PhasePush {
		t.Fatalf("phase after push = %s", j.Status().Phase)
	}
	j.Apply(ctx, tr)
	if j.Status().Phase != PhaseDone {
		t.Fatalf("phase after apply = %s", j.Status().Phase)
	}
}

func TestCancelledContextStopsRetrying(t *testing.T) {
	tr := newFake()
	tr.pushErr[2] = errors.New("reset")

	j, err := Plan(fixture(), tr, Options{Attempts: 5, Backoff: time.Hour})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	j.Push(ctx, tr, nil)

	if n := stateOf(j.Status(), 2); n.State != StateFailed {
		t.Fatalf("node 2 is %s after a cancelled push", n.State)
	}
	// One attempt, then the cancelled context cuts the wait short instead of
	// sitting on an hour-long backoff.
	if got := tr.pushCount(2); got != 1 {
		t.Fatalf("a cancelled push made %d attempts", got)
	}
}
