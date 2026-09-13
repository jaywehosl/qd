package store

import (
	"database/sql"
	"errors"
)

type Revision struct {
	Number      int
	CreatedAt   int64
	PublishedAt int64
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
