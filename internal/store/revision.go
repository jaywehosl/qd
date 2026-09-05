package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jaywehosl/quic-diver/internal/netstate"
)

var ErrNothingToPublish = errors.New("store: no draft to publish")

// Revision numbering has one rule: the highest number is the draft unless it
// has a published_at, in which case the draft does not exist yet and the next
// edit creates it.
type Revision struct {
	Number      int
	CreatedAt   int64
	PublishedAt int64
}

// Draft returns the unpublished revision, or nil when the tables match what the
// nodes are already running.
func (d *DB) Draft() (*Revision, error) {
	r, err := d.topRevision()
	if err != nil || r == nil || r.PublishedAt != 0 {
		return nil, err
	}
	return r, nil
}

// LastPublished is what a discard rewinds to.
func (d *DB) LastPublished() (*Revision, error) {
	var r Revision
	err := d.sql.QueryRow(
		`SELECT number, created_at, published_at FROM revisions
		  WHERE published_at != 0 ORDER BY number DESC LIMIT 1`,
	).Scan(&r.Number, &r.CreatedAt, &r.PublishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) topRevision() (*Revision, error) {
	var r Revision
	err := d.sql.QueryRow(
		`SELECT number, created_at, published_at FROM revisions ORDER BY number DESC LIMIT 1`,
	).Scan(&r.Number, &r.CreatedAt, &r.PublishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Touch opens a draft if none is open. Every configuration write calls it, so
// the revision number tracks "differs from what the nodes hold" rather than
// "somebody clicked something".
func (d *DB) Touch(now int64) (int, error) {
	top, err := d.topRevision()
	if err != nil {
		return 0, err
	}
	if top != nil && top.PublishedAt == 0 {
		return top.Number, nil
	}
	next := 1
	if top != nil {
		next = top.Number + 1
	}
	if _, err := d.sql.Exec(
		`INSERT INTO revisions (number, created_at, published_at, state_json) VALUES (?, ?, 0, '')`,
		next, now,
	); err != nil {
		return 0, err
	}
	return next, nil
}

// Publish freezes the current configuration into the draft revision and marks
// it published. The snapshot is the whole configuration half rather than the
// per-node projections: those are lossy — they carry no tags or comments — so
// they could never be replayed back into the tables by a discard or a rollback.
func (d *DB) Publish(now int64) (*netstate.State, error) {
	draft, err := d.Draft()
	if err != nil {
		return nil, err
	}
	if draft == nil {
		return nil, ErrNothingToPublish
	}

	state, err := d.LoadState()
	if err != nil {
		return nil, err
	}
	state.Revision = draft.Number

	blob, err := marshalState(state)
	if err != nil {
		return nil, err
	}
	if _, err := d.sql.Exec(
		`UPDATE revisions SET state_json = ?, published_at = ? WHERE number = ?`,
		blob, now, draft.Number,
	); err != nil {
		return nil, err
	}
	return state, nil
}

// Discard throws the draft away and puts the configuration tables back to the
// last published revision.
//
// Telemetry is untouched, and that is not incidental: the restore below deletes
// every configuration row before reinserting, so a foreign key from any
// telemetry table would take the collected statistics with it. See the note at
// the top of schema.sql.
func (d *DB) Discard() error {
	draft, err := d.Draft()
	if err != nil || draft == nil {
		return err
	}

	pub, err := d.LastPublished()
	if err != nil {
		return err
	}

	var state netstate.State
	if pub != nil {
		var blob string
		if err := d.sql.QueryRow(`SELECT state_json FROM revisions WHERE number = ?`, pub.Number).Scan(&blob); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(blob), &state); err != nil {
			return fmt.Errorf("store: revision %d snapshot is unreadable: %w", pub.Number, err)
		}
	}
	// With no published revision the nodes hold nothing, so "back to what they
	// have" is an empty configuration — which the zero State already is.

	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := restoreConfig(tx, &state); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM revisions WHERE published_at = 0`); err != nil {
		return err
	}
	return tx.Commit()
}

// RevisionState reads back a published snapshot. This is the only way to see
// what the nodes are actually running: the live tables are the draft and have
// already moved on.
func (d *DB) RevisionState(number int) (*netstate.State, error) {
	var blob string
	err := d.sql.QueryRow(
		`SELECT state_json FROM revisions WHERE number = ? AND published_at != 0`, number,
	).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("store: revision %d was never published", number)
	}
	if err != nil {
		return nil, err
	}
	var s netstate.State
	if err := json.Unmarshal([]byte(blob), &s); err != nil {
		return nil, fmt.Errorf("store: revision %d snapshot is unreadable: %w", number, err)
	}
	return &s, nil
}

// RecordNodeProgress writes what a node is running into the telemetry half.
//
// Telemetry, not configuration: it must not be rewound by a discard, and it
// only moves forward — a stale report arriving after a newer one must not drag
// the record backwards.
func (d *DB) RecordNodeProgress(nodeID, applied, staged int, status string, now int64) error {
	_, err := d.sql.Exec(`
		INSERT INTO node_state (node_id, applied_revision, staged_revision, status, last_seen)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET
			applied_revision = MAX(node_state.applied_revision, excluded.applied_revision),
			staged_revision  = MAX(node_state.staged_revision,  excluded.staged_revision),
			status           = excluded.status,
			last_seen        = excluded.last_seen`,
		nodeID, applied, staged, status, now)
	return err
}

// NodeProgress is what the nodes list shows as drift.
type NodeProgress struct {
	NodeID   int
	Applied  int
	Staged   int
	Status   string
	LastSeen int64
}

func (d *DB) NodeProgress() (map[int]NodeProgress, error) {
	out := map[int]NodeProgress{}
	err := scan(d.sql, `SELECT node_id, applied_revision, staged_revision, status, last_seen FROM node_state`,
		func(r *sql.Rows) error {
			var p NodeProgress
			if err := r.Scan(&p.NodeID, &p.Applied, &p.Staged, &p.Status, &p.LastSeen); err != nil {
				return err
			}
			out[p.NodeID] = p
			return nil
		})
	return out, err
}

// Rollback republishes an earlier revision by restoring its snapshot into the
// tables as a fresh draft. It does not resurrect the old revision number: what
// the nodes ran at 31 and what they will run now are different events even when
// the bytes match.
func (d *DB) Rollback(number int, now int64) error {
	var blob string
	err := d.sql.QueryRow(
		`SELECT state_json FROM revisions WHERE number = ? AND published_at != 0`, number,
	).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("store: revision %d was never published", number)
	}
	if err != nil {
		return err
	}

	var state netstate.State
	if err := json.Unmarshal([]byte(blob), &state); err != nil {
		return fmt.Errorf("store: revision %d snapshot is unreadable: %w", number, err)
	}

	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := restoreConfig(tx, &state); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM revisions WHERE published_at = 0`); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = d.Touch(now)
	return err
}

// restoreConfig replaces the configuration half wholesale. Order matters on the
// way out (children first) and on the way back in (parents first) because the
// foreign keys within the configuration half are real.
func restoreConfig(tx *sql.Tx, s *netstate.State) error {
	for _, q := range []string{
		`DELETE FROM group_entrypoints`,
		`DELETE FROM clients`,
		`DELETE FROM groups`,
		`DELETE FROM entrypoints`,
		`DELETE FROM nodes`,
		`DELETE FROM network`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return fmt.Errorf("store: %s: %w", q, err)
		}
	}

	for _, n := range s.Nodes {
		if _, err := tx.Exec(
			`INSERT INTO nodes (id, tag, address, port, role, enable, uuid,
			                    dns_primary, dns_secondary, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
			n.ID, n.Tag, n.Address, n.Port, string(n.Role), n.Enable, n.UUID,
			n.DNSPrimary, n.DNSSecondary,
		); err != nil {
			return fmt.Errorf("store: restoring node %d: %w", n.ID, err)
		}
	}
	for _, e := range s.Entrypoints {
		if _, err := tx.Exec(
			`INSERT INTO entrypoints (id, node_id, port, remark, enable, created_at)
			 VALUES (?, ?, ?, ?, ?, 0)`,
			e.ID, e.NodeID, e.Port, e.Remark, e.Enable,
		); err != nil {
			return fmt.Errorf("store: restoring entrypoint %d: %w", e.ID, err)
		}
	}
	for _, g := range s.Groups {
		if _, err := tx.Exec(
			`INSERT INTO groups (id, tag, allow_exit, created_at) VALUES (?, ?, ?, 0)`,
			g.ID, g.Tag, g.AllowExit,
		); err != nil {
			return fmt.Errorf("store: restoring group %d: %w", g.ID, err)
		}
		for _, id := range g.EntrypointIDs {
			if _, err := tx.Exec(
				`INSERT INTO group_entrypoints (group_id, entrypoint_id) VALUES (?, ?)`, g.ID, id,
			); err != nil {
				return fmt.Errorf("store: restoring group %d membership: %w", g.ID, err)
			}
		}
	}
	for _, c := range s.Clients {
		var group any
		if c.GroupID != 0 {
			group = c.GroupID
		}
		if _, err := tx.Exec(
			`INSERT INTO clients (id, tag, uuid, group_id, enable, expiry_at, comment, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
			c.ID, c.Tag, c.UUID, group, c.Enable, c.ExpiryAt, c.Comment,
		); err != nil {
			return fmt.Errorf("store: restoring client %d: %w", c.ID, err)
		}
	}
	if s.NetworkKey != "" {
		if _, err := tx.Exec(
			`INSERT INTO network (id, key, created_at) VALUES (1, ?, 0)
			 ON CONFLICT(id) DO UPDATE SET key = excluded.key`, s.NetworkKey); err != nil {
			return fmt.Errorf("store: restoring the network key: %w", err)
		}
	}
	return nil
}

func (d *DB) Version() (int, error) {
	var n int
	err := d.sql.QueryRow(`SELECT COALESCE(MAX(number), 0) FROM revisions`).Scan(&n)
	return n, err
}

func (d *DB) Bump(now int64) (int, error) {
	if _, err := d.sql.Exec(
		`INSERT INTO revisions (number, created_at, published_at, state_json)
		 SELECT COALESCE(MAX(number), 0) + 1, ?, ?, '' FROM revisions`, now, now); err != nil {
		return 0, err
	}
	return d.Version()
}

func (d *DB) Settle(number int, now int64) (int, error) {
	if number <= 0 {
		return d.Bump(now)
	}
	if _, err := d.sql.Exec(
		`INSERT INTO revisions (number, created_at, published_at, state_json)
		 VALUES (?, ?, ?, '')
		 ON CONFLICT(number) DO UPDATE SET published_at = excluded.published_at`,
		number, now, now); err != nil {
		return 0, err
	}
	return d.Version()
}
