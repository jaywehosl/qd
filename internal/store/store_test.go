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
}
