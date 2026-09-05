package store

import (
	"path/filepath"
	"testing"

	"github.com/jaywehosl/quic-diver/internal/netstate"
)

func open(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// A minimal but complete network: one node, one entrypoint, one group holding
// it, one client in that group.
func seed(t *testing.T, d *DB) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.SQL().Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO nodes (id, tag, address, port, role, enable, created_at)
	      VALUES (1, 'in-1', '1.2.3.4', 443, 'ingress', 1, 1)`)
	exec(`INSERT INTO entrypoints (id, node_id, port, enable, created_at) VALUES (10, 1, 443, 1, 1)`)
	exec(`INSERT INTO groups (id, tag, allow_exit, created_at) VALUES (100, 'russia-in', 1, 1)`)
	exec(`INSERT INTO group_entrypoints (group_id, entrypoint_id) VALUES (100, 10)`)
	exec(`INSERT INTO clients (id, tag, uuid, group_id, enable, created_at)
	      VALUES (1000, 'vasya', 'uuid-vasya', 100, 1, 1)`)
	exec(`INSERT INTO network (id, key, created_at) VALUES (1, 'admin-working', 1)`)
}

func seedTelemetry(t *testing.T, d *DB) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.SQL().Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO client_traffic (client_id, node_id, epoch, up, down, at)
	      VALUES (1000, 1, 7, 2147483648, 31138512896, 1735689600000)`)
	exec(`INSERT INTO devices (client_id, fingerprint, platform, last_seen)
	      VALUES (1000, 'fp-desktop', 'windows', 1735689600000)`)
	exec(`INSERT INTO ip_log (client_id, fingerprint, ip, last_seen)
	      VALUES (1000, 'fp-desktop', '203.0.113.9', 1735689600000)`)
	exec(`INSERT INTO node_metrics (node_id, metric, t, v) VALUES (1, 'cpu', 1735689600, 12.5)`)
	exec(`INSERT INTO node_state (node_id, applied_revision, status, last_seen)
	      VALUES (1, 1, 'online', 1735689600000)`)
}

func count(t *testing.T, d *DB, table string) int {
	t.Helper()
	var n int
	if err := d.SQL().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return n
}

func TestSchemaApplies(t *testing.T) {
	d := open(t)
	for _, table := range []string{
		"nodes", "entrypoints", "groups", "group_entrypoints", "clients",
		"revisions", "network",
		"node_state", "client_traffic", "devices", "ip_log", "node_metrics",
	} {
		if got := count(t, d, table); got != 0 {
			t.Fatalf("fresh %s has %d rows", table, got)
		}
	}
}

// Re-opening must not fail on the second CREATE, or every restart is a crash.
func TestSchemaIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	for i := 0; i < 3; i++ {
		d, err := Open(path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		d.Close()
	}
}

func TestLoadStateFeedsTheProjection(t *testing.T) {
	d := open(t)
	seed(t, d)
	if _, err := d.Touch(1); err != nil {
		t.Fatal(err)
	}

	s, err := d.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := netstate.Project(1, s)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(cfg.Clients) != 1 || cfg.Clients[0].UUID != "uuid-vasya" {
		t.Fatalf("clients = %+v", cfg.Clients)
	}
	if !cfg.Clients[0].AllowExit {
		t.Fatal("allow_exit did not survive the trip through the database")
	}
	if len(cfg.Ents) != 1 || cfg.Ents[0].Port != 443 {
		t.Fatalf("entrypoints = %+v", cfg.Ents)
	}
}

// The invariant the schema is shaped around: a discard rewrites every
// configuration row, and must not cost a single byte of collected statistics.
func TestDiscardKeepsTelemetry(t *testing.T) {
	d := open(t)
	seed(t, d)
	seedTelemetry(t, d)

	if _, err := d.Touch(1); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Publish(2); err != nil {
		t.Fatal(err)
	}

	// A draft that deletes the client outright — the harshest case, because a
	// cascade would fire on exactly this row.
	if _, err := d.Touch(3); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQL().Exec(`DELETE FROM clients WHERE id = 1000`); err != nil {
		t.Fatal(err)
	}
	if got := count(t, d, "clients"); got != 0 {
		t.Fatalf("client survived its own deletion: %d", got)
	}

	if err := d.Discard(); err != nil {
		t.Fatalf("Discard: %v", err)
	}

	if got := count(t, d, "clients"); got != 1 {
		t.Fatalf("discard did not bring the client back: %d rows", got)
	}
	for _, table := range []string{"client_traffic", "devices", "ip_log", "node_metrics", "node_state"} {
		if got := count(t, d, table); got != 1 {
			t.Fatalf("discard destroyed %s: %d rows left", table, got)
		}
	}

	var up, down int64
	if err := d.SQL().QueryRow(`SELECT up, down FROM client_traffic WHERE client_id = 1000`).Scan(&up, &down); err != nil {
		t.Fatalf("traffic row: %v", err)
	}
	if up != 2147483648 || down != 31138512896 {
		t.Fatalf("counters changed: up=%d down=%d", up, down)
	}
}

// Runtime state lives outside `nodes` precisely so a discard cannot rewind it:
// what a node is actually running is a fact, not a draft.
func TestDiscardDoesNotRewindWhatNodesAreRunning(t *testing.T) {
	d := open(t)
	seed(t, d)
	if _, err := d.Touch(1); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Publish(2); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQL().Exec(
		`INSERT INTO node_state (node_id, applied_revision, status) VALUES (1, 1, 'online')`); err != nil {
		t.Fatal(err)
	}

	if _, err := d.Touch(3); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQL().Exec(`UPDATE nodes SET port = 8443 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if err := d.Discard(); err != nil {
		t.Fatal(err)
	}

	var applied int
	var status string
	if err := d.SQL().QueryRow(
		`SELECT applied_revision, status FROM node_state WHERE node_id = 1`).Scan(&applied, &status); err != nil {
		t.Fatal(err)
	}
	if applied != 1 || status != "online" {
		t.Fatalf("node_state was rolled back with the draft: rev=%d status=%q", applied, status)
	}

	var port int
	if err := d.SQL().QueryRow(`SELECT port FROM nodes WHERE id = 1`).Scan(&port); err != nil {
		t.Fatal(err)
	}
	if port != 443 {
		t.Fatalf("the draft edit survived the discard: port=%d", port)
	}
}

func TestDraftOpensOnlyOnce(t *testing.T) {
	d := open(t)
	seed(t, d)

	a, err := d.Touch(1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := d.Touch(2)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("a second edit opened revision %d on top of %d", b, a)
	}

	if _, err := d.Publish(3); err != nil {
		t.Fatal(err)
	}
	if draft, err := d.Draft(); err != nil || draft != nil {
		t.Fatalf("a draft is still open after publishing: %+v (%v)", draft, err)
	}

	c, err := d.Touch(4)
	if err != nil {
		t.Fatal(err)
	}
	if c != a+1 {
		t.Fatalf("the edit after a publish opened %d, want %d", c, a+1)
	}
}

func TestPublishRefusesWithoutADraft(t *testing.T) {
	d := open(t)
	seed(t, d)
	if _, err := d.Publish(1); err == nil {
		t.Fatal("published a revision that does not exist")
	}
}

// With nothing ever published the nodes hold nothing, so a discard means an
// empty configuration rather than "keep whatever is lying around".
func TestDiscardBeforeAnyPublishEmptiesTheDraft(t *testing.T) {
	d := open(t)
	seed(t, d)
	seedTelemetry(t, d)
	if _, err := d.Touch(1); err != nil {
		t.Fatal(err)
	}

	if err := d.Discard(); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"nodes", "entrypoints", "groups", "clients", "network"} {
		if got := count(t, d, table); got != 0 {
			t.Fatalf("%s kept %d rows with nothing published", table, got)
		}
	}
	if got := count(t, d, "client_traffic"); got != 1 {
		t.Fatalf("telemetry was destroyed anyway: %d rows", got)
	}
}

func TestRollbackRestoresAnEarlierRevisionAsAFreshDraft(t *testing.T) {
	d := open(t)
	seed(t, d)
	if _, err := d.Touch(1); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Publish(2); err != nil { // revision 1: port 443
		t.Fatal(err)
	}

	if _, err := d.Touch(3); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQL().Exec(`UPDATE nodes SET port = 8443 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Publish(4); err != nil { // revision 2: port 8443
		t.Fatal(err)
	}

	if err := d.Rollback(1, 5); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	var port int
	if err := d.SQL().QueryRow(`SELECT port FROM nodes WHERE id = 1`).Scan(&port); err != nil {
		t.Fatal(err)
	}
	if port != 443 {
		t.Fatalf("rollback did not restore revision 1: port=%d", port)
	}

	// A rollback is a new event, not a resurrection of the old number.
	draft, err := d.Draft()
	if err != nil || draft == nil {
		t.Fatalf("rollback left no draft to publish: %+v (%v)", draft, err)
	}
	if draft.Number != 3 {
		t.Fatalf("rollback opened revision %d, want 3", draft.Number)
	}
}

func TestRollbackRefusesAnUnpublishedRevision(t *testing.T) {
	d := open(t)
	seed(t, d)
	if _, err := d.Touch(1); err != nil {
		t.Fatal(err)
	}
	if err := d.Rollback(1, 2); err == nil {
		t.Fatal("rolled back to a revision no node ever ran")
	}
}

// A published snapshot has to survive the round trip, or a discard silently
// restores something other than what was published.
func TestSnapshotRoundTripsEverything(t *testing.T) {
	d := open(t)
	seed(t, d)
	if _, err := d.SQL().Exec(
		`UPDATE clients SET comment = 'paid until March', expiry_at = 1735689600000 WHERE id = 1000`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Touch(1); err != nil {
		t.Fatal(err)
	}
	before, err := d.Publish(2)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := d.Touch(3); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQL().Exec(`DELETE FROM nodes`); err != nil {
		t.Fatal(err)
	}
	if err := d.Discard(); err != nil {
		t.Fatal(err)
	}

	after, err := d.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Nodes) != len(before.Nodes) || len(after.Clients) != len(before.Clients) {
		t.Fatalf("shape changed: %d/%d nodes, %d/%d clients",
			len(after.Nodes), len(before.Nodes), len(after.Clients), len(before.Clients))
	}
	if after.Clients[0].Comment != "paid until March" {
		t.Fatalf("comment did not survive: %q", after.Clients[0].Comment)
	}
	if after.Clients[0].ExpiryAt != 1735689600000 {
		t.Fatalf("expiry did not survive: %d", after.Clients[0].ExpiryAt)
	}
	if len(after.Groups) != 1 || len(after.Groups[0].EntrypointIDs) != 1 {
		t.Fatalf("group membership did not survive: %+v", after.Groups)
	}
	if after.NetworkKey == "" {
		t.Fatal("the network key did not survive the rollback")
	}
}
