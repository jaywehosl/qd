package store

import (
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
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
		`UPDATE network SET max_streams = 65536 WHERE max_streams = 4096`,
	} {
		if _, err := h.Exec(add); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			h.Close()
			return nil, fmt.Errorf("store: %s: %w", add, err)
		}
	}
	return &DB{sql: h}, nil
}

// OpenRead открывает базу, не трогая схему. Живой узел держит её открытой, и
// миграция из читающей команды упирается в SQLITE_BUSY: -status тогда молчал
// про личность узла и печатал прочерки на исправной машине.
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
	s := &netstate.State{
		Nodes:       []netstate.Node{},
		Entrypoints: []netstate.Entrypoint{},
		Groups:      []netstate.Group{},
		Clients:     []netstate.Client{},
	}

	if err := d.sql.QueryRow(`SELECT COALESCE(MAX(number), 0) FROM revisions`).Scan(&s.Revision); err != nil {
		return nil, err
	}

	if err := scan(d.sql, `SELECT id, tag, address, port, role, enable, uuid, dns_primary, dns_secondary FROM nodes ORDER BY id`,
		func(r *sql.Rows) error {
			var n netstate.Node
			if err := r.Scan(&n.ID, &n.Tag, &n.Address, &n.Port, &n.Role,
				&n.Enable, &n.UUID, &n.DNSPrimary, &n.DNSSecondary); err != nil {
				return err
			}
			s.Nodes = append(s.Nodes, n)
			return nil
		}); err != nil {
		return nil, err
	}

	if err := scan(d.sql, `SELECT id, node_id, port, remark, enable FROM entrypoints ORDER BY id`,
		func(r *sql.Rows) error {
			var e netstate.Entrypoint
			if err := r.Scan(&e.ID, &e.NodeID, &e.Port, &e.Remark, &e.Enable); err != nil {
				return err
			}
			s.Entrypoints = append(s.Entrypoints, e)
			return nil
		}); err != nil {
		return nil, err
	}

	byGroup := map[int][]int{}
	if err := scan(d.sql, `SELECT group_id, entrypoint_id FROM group_entrypoints ORDER BY group_id, entrypoint_id`,
		func(r *sql.Rows) error {
			var g, e int
			if err := r.Scan(&g, &e); err != nil {
				return err
			}
			byGroup[g] = append(byGroup[g], e)
			return nil
		}); err != nil {
		return nil, err
	}

	if err := scan(d.sql, `SELECT id, tag, allow_exit, device_limit FROM groups ORDER BY id`,
		func(r *sql.Rows) error {
			var g netstate.Group
			if err := r.Scan(&g.ID, &g.Tag, &g.AllowExit, &g.DeviceLimit); err != nil {
				return err
			}
			g.EntrypointIDs = byGroup[g.ID]
			s.Groups = append(s.Groups, g)
			return nil
		}); err != nil {
		return nil, err
	}

	if err := scan(d.sql, `SELECT id, tag, uuid, COALESCE(group_id, 0), enable, expiry_at, comment, admin, device_limit, allow_exit FROM clients ORDER BY id`,
		func(r *sql.Rows) error {
			var c netstate.Client
			if err := r.Scan(&c.ID, &c.Tag, &c.UUID, &c.GroupID, &c.Enable, &c.ExpiryAt, &c.Comment,
				&c.Admin, &c.DeviceLimit, &c.AllowExit); err != nil {
				return err
			}
			s.Clients = append(s.Clients, c)
			return nil
		}); err != nil {
		return nil, err
	}

	if err := d.sql.QueryRow(`SELECT key FROM network WHERE id = 1`).Scan(&s.NetworkKey); err != nil &&
		!errors.Is(err, sql.ErrNoRows) {
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

func marshalState(s *netstate.State) (string, error) {
	blob, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(blob), nil
}
