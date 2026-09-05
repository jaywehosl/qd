package clientstate

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type DB struct {
	sql *sql.DB
}

func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}

	handle, err := sql.Open("sqlite", path+
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate")
	if err != nil {
		return nil, err
	}

	handle.SetMaxOpenConns(1)

	if _, err := handle.Exec(schema); err != nil {
		handle.Close()
		return nil, fmt.Errorf("clientstate: schema: %w", err)
	}
	return &DB{sql: handle}, nil
}

func (d *DB) Close() error { return d.sql.Close() }
func (d *DB) SQL() *sql.DB { return d.sql }

type Subscription struct {
	URI         string
	Key         string
	Label       string
	Tag         string
	Admin       bool
	AllowExit   bool
	ExpiresAt   int64
	CreatedAt   int64
	LastRefresh int64
	Imported    bool
}

func (d *DB) Subscription() (Subscription, error) {
	var s Subscription
	err := d.sql.QueryRow(
		`SELECT uri, key, label, tag, admin, allow_exit, expires_at, created_at, last_refresh
		   FROM subscription WHERE id = 1`,
	).Scan(&s.URI, &s.Key, &s.Label, &s.Tag, &s.Admin, &s.AllowExit, &s.ExpiresAt, &s.CreatedAt, &s.LastRefresh)
	if errors.Is(err, sql.ErrNoRows) {
		return Subscription{}, nil
	}
	if err != nil {
		return Subscription{}, err
	}
	s.Imported = s.Key != ""
	return s, nil
}

func (d *DB) SaveSubscription(s Subscription) error {
	_, err := d.sql.Exec(`
		INSERT INTO subscription (id, uri, key, label, tag, admin, allow_exit, expires_at, created_at, last_refresh)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			uri = excluded.uri, key = excluded.key, label = excluded.label,
			tag = excluded.tag, admin = excluded.admin, allow_exit = excluded.allow_exit,
			expires_at = excluded.expires_at, created_at = excluded.created_at,
			last_refresh = excluded.last_refresh`,
		s.URI, s.Key, s.Label, s.Tag, s.Admin, s.AllowExit, s.ExpiresAt, s.CreatedAt, s.LastRefresh)
	return err
}

func (d *DB) ClearSubscription() error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM subscription`,
		`DELETE FROM nodes`,
		`DELETE FROM notifications`,
		`DELETE FROM samples`,
		`DELETE FROM traffic`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type Node struct {
	ID        int
	Name      string
	Role      string
	Address   string
	Port      int
	LatencyMs int
	Reachable bool
	Selected  bool
}

func (d *DB) Nodes() ([]Node, error) {
	rows, err := d.sql.Query(
		`SELECT id, name, role, address, port, latency_ms, reachable, selected FROM nodes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Node{}
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Name, &n.Role, &n.Address, &n.Port,
			&n.LatencyMs, &n.Reachable, &n.Selected); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (d *DB) ReplaceNodes(nodes []Node) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM nodes`); err != nil {
		return err
	}
	for _, n := range nodes {
		if _, err := tx.Exec(
			`INSERT INTO nodes (id, name, role, address, port, latency_ms, reachable, selected)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			n.ID, n.Name, n.Role, n.Address, n.Port, n.LatencyMs, n.Reachable, n.Selected); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) MarkNode(id, latencyMs int, reachable, selected bool) error {
	_, err := d.sql.Exec(
		`UPDATE nodes SET latency_ms = ?, reachable = ?, selected = ? WHERE id = ?`,
		latencyMs, reachable, selected, id)
	return err
}

func (d *DB) MarkReach(id, latencyMs int, reachable bool) error {
	_, err := d.sql.Exec(
		`UPDATE nodes SET latency_ms = ?, reachable = ? WHERE id = ?`,
		latencyMs, reachable, id)
	return err
}

func (d *DB) ClearSelection() error {
	_, err := d.sql.Exec(`UPDATE nodes SET selected = 0`)
	return err
}

type Settings struct {
	RefreshMinutes     int    `json:"refreshMinutes"`
	RefreshPinned      bool   `json:"refreshPinned"`
	Autostart          bool   `json:"autostart"`
	AutostartBehaviour string `json:"autostartBehaviour"`
	ManualBehaviour    string `json:"manualBehaviour"`

	NetworkKey string `json:"-"`

	Egress  bool `json:"-"`
	Adblock bool `json:"-"`

	FixedRate  int  `json:"fixedRate"`
	RatePinned bool `json:"ratePinned"`
}

func DefaultSettings() Settings {
	return Settings{
		RefreshMinutes:     60,
		RefreshPinned:      false,
		Autostart:          false,
		AutostartBehaviour: "tray",
		ManualBehaviour:    "open",
		Egress:             false,
		Adblock:            false,
		FixedRate:          0,
		RatePinned:         false,
	}
}

func (d *DB) Settings() (Settings, error) {
	s := DefaultSettings()

	rows, err := d.sql.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return s, err
	}
	defer rows.Close()

	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return s, err
		}
		switch k {
		case "refreshMinutes":
			s.RefreshMinutes = atoiOr(v, s.RefreshMinutes)
		case "refreshPinned":
			s.RefreshPinned = v == "1"
		case "autostart":
			s.Autostart = v == "1"
		case "autostartBehaviour":
			s.AutostartBehaviour = v
		case "manualBehaviour":
			s.ManualBehaviour = v
		case "networkKey":
			s.NetworkKey = v
		case "egress":
			s.Egress = v == "1"
		case "adblock":
			s.Adblock = v == "1"
		case "fixedRate":
			s.FixedRate = atoiOr(v, 0)
		case "ratePinned":
			s.RatePinned = v == "1"
		}
	}
	return s, rows.Err()
}

func (d *DB) SaveSettings(s Settings) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for k, v := range map[string]string{
		"refreshMinutes":     strconv.Itoa(s.RefreshMinutes),
		"refreshPinned":      boolText(s.RefreshPinned),
		"autostart":          boolText(s.Autostart),
		"autostartBehaviour": s.AutostartBehaviour,
		"manualBehaviour":    s.ManualBehaviour,
		"networkKey":         s.NetworkKey,
		"egress":             boolText(s.Egress),
		"adblock":            boolText(s.Adblock),
		"fixedRate":          strconv.Itoa(s.FixedRate),
		"ratePinned":         boolText(s.RatePinned),
	} {
		if _, err := tx.Exec(
			`INSERT INTO settings (key, value) VALUES (?, ?)
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) Value(key string) (string, error) {
	var value string
	err := d.sql.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (d *DB) SetValue(key, value string) error {
	_, err := d.sql.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (d *DB) ResetSettings() error {
	if _, err := d.sql.Exec(`DELETE FROM settings`); err != nil {
		return err
	}
	if _, err := d.sql.Exec(`DELETE FROM rules`); err != nil {
		return err
	}
	_, err := d.sql.Exec(`DELETE FROM sites`)
	return err
}

func atoiOr(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func boolText(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
