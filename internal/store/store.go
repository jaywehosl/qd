package store

import (
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/jaywehosl/quic-diver/internal/netstate"
)

//go:embed schema.sql
var schema string

type DB struct {
	sql *sql.DB
}

func Open(path string) (*DB, error) {
	h, err := sql.Open("sqlite", path+
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	if _, err := h.Exec(schema); err != nil {
		h.Close()
		return nil, fmt.Errorf("store: applying schema: %w", err)
	}
	for _, add := range []string{
		`ALTER TABLE clients ADD COLUMN admin INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE clients ADD COLUMN device_limit INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE devices ADD COLUMN model TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE devices ADD COLUMN kind TEXT NOT NULL DEFAULT 'desktop'`,
		`ALTER TABLE devices ADD COLUMN blocked INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE groups ADD COLUMN device_limit INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE groups ADD COLUMN relay_enable INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE groups ADD COLUMN relays TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE clients ADD COLUMN allow_exit INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE network ADD COLUMN refresh_minutes INTEGER NOT NULL DEFAULT 480`,
		`ALTER TABLE nodes ADD COLUMN uuid TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN dns_primary TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN dns_secondary TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE network ADD COLUMN dns_primary TEXT NOT NULL DEFAULT '1.1.1.1'`,
		`ALTER TABLE network ADD COLUMN dns_secondary TEXT NOT NULL DEFAULT '8.8.8.8'`,
		`ALTER TABLE network ADD COLUMN dns_cache INTEGER NOT NULL DEFAULT 4096`,
		`ALTER TABLE network ADD COLUMN dns_min_ttl INTEGER NOT NULL DEFAULT 60`,
		`ALTER TABLE network ADD COLUMN dns_max_ttl INTEGER NOT NULL DEFAULT 3600`,
		`ALTER TABLE network ADD COLUMN dns_stale INTEGER NOT NULL DEFAULT 60`,
		`ALTER TABLE network ADD COLUMN mtu INTEGER NOT NULL DEFAULT 1500`,
		`ALTER TABLE network ADD COLUMN stats_seconds INTEGER NOT NULL DEFAULT 5`,
		`ALTER TABLE network ADD COLUMN pool TEXT NOT NULL DEFAULT '10.7.0.0/16'`,
		`ALTER TABLE network ADD COLUMN brutal_mbit INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE network ADD COLUMN max_streams INTEGER NOT NULL DEFAULT 65536`,
		`ALTER TABLE network ADD COLUMN stream_window INTEGER NOT NULL DEFAULT 2048`,
		`ALTER TABLE network ADD COLUMN max_stream_window INTEGER NOT NULL DEFAULT 6144`,
		`ALTER TABLE network ADD COLUMN conn_window INTEGER NOT NULL DEFAULT 3072`,
		`ALTER TABLE network ADD COLUMN max_conn_window INTEGER NOT NULL DEFAULT 15360`,
		`ALTER TABLE network ADD COLUMN idle_seconds INTEGER NOT NULL DEFAULT 90`,
		`ALTER TABLE network ADD COLUMN keepalive_seconds INTEGER NOT NULL DEFAULT 15`,
		`ALTER TABLE network ADD COLUMN socket_buffer INTEGER NOT NULL DEFAULT 2048`,
		`ALTER TABLE nodes ADD COLUMN authority TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN cert_path TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN key_path TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE groups ADD COLUMN route_dns INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE network ADD COLUMN route_list TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE network ADD COLUMN route_services TEXT NOT NULL DEFAULT ''`,
		`UPDATE network SET max_streams = 65536 WHERE max_streams = 4096`,
	} {
		if _, err := h.Exec(add); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			h.Close()
			return nil, fmt.Errorf("store: %s: %w", add, err)
		}
	}
	return &DB{sql: h}, nil
}

func OpenRead(path string) (*DB, error) {
	h, err := sql.Open("sqlite", path+
		"?_pragma=foreign_keys(1)&_pragma=busy_timeout(2000)&mode=ro")
	if err != nil {
		return nil, err
	}
	if err := h.Ping(); err != nil {
		h.Close()
		return nil, err
	}
	return &DB{sql: h}, nil
}

func (d *DB) Close() error { return d.sql.Close() }

func (d *DB) SQL() *sql.DB { return d.sql }

func (d *DB) LoadState() (*netstate.State, error) {
	s := &netstate.State{}
	var err error
	if s.Nodes, err = d.Nodes(); err != nil {
		return nil, err
	}
	if s.Entrypoints, err = d.Entrypoints(); err != nil {
		return nil, err
	}
	if s.Groups, err = d.Groups(); err != nil {
		return nil, err
	}
	if s.Clients, err = d.Clients(); err != nil {
		return nil, err
	}
	return s, nil
}

func (d *DB) NetworkKey(now int64) (string, error) {
	var key string
	err := d.sql.QueryRow(`SELECT key FROM network WHERE id = 1`).Scan(&key)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	key = hex.EncodeToString(raw)

	if _, err := d.sql.Exec(
		`INSERT INTO network (id, key, created_at) VALUES (1, ?, ?)`, key, now); err != nil {
		return "", err
	}
	return key, nil
}

func (d *DB) SetNetworkKey(key string, now int64) error {
	_, err := d.sql.Exec(
		`INSERT INTO network (id, key, created_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET key = excluded.key`, key, now)
	return err
}

func scan(h *sql.DB, query string, fn func(*sql.Rows) error, args ...any) error {
	rows, err := h.Query(query, args...)
	if err != nil {
		return fmt.Errorf("store: %s: %w", query, err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
