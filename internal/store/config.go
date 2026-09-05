package store

import (
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/jaywehosl/quic-diver/internal/netstate"
)

var ErrNotFound = errors.New("store: no such row")

var ErrExitEntrypoint = errors.New("an exit node's entrypoint cannot be given to a group: clients would be handed its address and authorised on it")

func (d *DB) SelfNode(want netstate.Node, address string, now int64) (netstate.Node, error) {
	rows, err := d.Nodes()
	if err != nil {
		return netstate.Node{}, err
	}
	if want.UUID != "" {
		for _, n := range rows {
			if n.UUID == want.UUID {
				return d.settle(n, want, now)
			}
		}
	}
	if want.ID > 0 {
		for _, n := range rows {
			if n.ID == want.ID && (n.UUID == "" || n.UUID == want.UUID) {
				n.UUID = want.UUID
				return d.settle(n, want, now)
			}
		}
	}
	for _, n := range rows {
		if n.Address == address || (want.Address != "" && n.Address == want.Address) {
			return d.settle(n, want, now)
		}
	}

	taken := map[string]bool{}
	for _, n := range rows {
		taken[n.Tag] = true
	}
	fresh := want
	if fresh.Address == "" {
		fresh.Address = address
	}
	if fresh.Port == 0 {
		fresh.Port = DefaultPort
	}
	if fresh.Role == "" {
		fresh.Role = netstate.RoleIngress
	}
	if fresh.Tag == "" {
		fresh.Tag = netstate.PickName(fresh.Role, taken)
	}
	fresh.Enable = true
	id, err := d.SaveNode(fresh, now)
	if err != nil {
		return netstate.Node{}, err
	}
	fresh.ID = id
	return fresh, nil
}

func (d *DB) SelfEntrypoint(self netstate.Node, now int64) error {
	held, err := d.Entrypoints()
	if err != nil {
		return err
	}
	for _, e := range held {
		if e.NodeID == self.ID {
			return nil
		}
	}
	_, err = d.SaveEntrypoint(netstate.Entrypoint{
		NodeID: self.ID,
		Port:   self.Port,
		Remark: self.Tag,
		Enable: true,
	}, now)
	return err
}

const DefaultPort = 51820

func (d *DB) Nodes() ([]netstate.Node, error) {
	out := []netstate.Node{}
	err := scan(d.sql, `SELECT id, tag, address, port, role, enable, uuid, dns_primary, dns_secondary, authority, cert_path, key_path FROM nodes ORDER BY id`,
		func(r *sql.Rows) error {
			var n netstate.Node
			if err := r.Scan(&n.ID, &n.Tag, &n.Address, &n.Port, &n.Role, &n.Enable, &n.UUID,
				&n.DNSPrimary, &n.DNSSecondary, &n.Authority, &n.CertPath, &n.KeyPath); err != nil {
				return err
			}
			out = append(out, n)
			return nil
		})
	return out, err
}

func (d *DB) nameNode(n netstate.Node) netstate.Node {
	held, err := d.Nodes()
	if err != nil {
		return n
	}

	taken := map[string]bool{}
	var before netstate.Node
	found := false
	for _, other := range held {
		if other.ID == n.ID && n.ID != 0 {
			before, found = other, true
			continue
		}
		if other.Tag != "" {
			taken[other.Tag] = true
		}
	}

	if n.Tag != "" && (!found || before.Role == n.Role) {
		return n
	}

	if picked := netstate.PickName(n.Role, taken); picked != "" {
		n.Tag = picked
	}
	return n
}

func (d *DB) SaveNode(n netstate.Node, now int64) (int, error) {
	n = d.nameNode(n)

	if n.ID == 0 {
		res, err := d.sql.Exec(
			`INSERT INTO nodes (tag, address, port, role, enable, uuid, dns_primary, dns_secondary, authority, cert_path, key_path, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			n.Tag, n.Address, n.Port, string(n.Role), n.Enable, n.UUID,
			n.DNSPrimary, n.DNSSecondary, n.Authority, n.CertPath, n.KeyPath, now)
		if err != nil {
			return 0, err
		}
		id, err := res.LastInsertId()
		return int(id), err
	}

	res, err := d.sql.Exec(
		`UPDATE nodes SET tag = ?, address = ?, port = ?, role = ?, enable = ?, uuid = ?,
		        dns_primary = ?, dns_secondary = ?, authority = ?, cert_path = ?, key_path = ? WHERE id = ?`,
		n.Tag, n.Address, n.Port, string(n.Role), n.Enable, n.UUID,
		n.DNSPrimary, n.DNSSecondary, n.Authority, n.CertPath, n.KeyPath, n.ID)
	if err != nil {
		return 0, err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		if err := d.InsertNodeAt(n, now); err != nil {
			return 0, err
		}
	}
	d.alignEntrypoints(n, now)
	return n.ID, nil
}

func (d *DB) alignEntrypoints(n netstate.Node, now int64) {
	if n.ID == 0 || n.Port <= 0 {
		return
	}
	held, err := d.Entrypoints()
	if err != nil {
		return
	}
	for _, e := range held {
		if e.NodeID != n.ID || e.Port == n.Port {
			continue
		}
		e.Port = n.Port
		d.SaveEntrypoint(e, now)
	}
}

func (d *DB) InsertNodeAt(n netstate.Node, now int64) error {
	_, err := d.sql.Exec(
		`INSERT INTO nodes (id, tag, address, port, role, enable, uuid, dns_primary, dns_secondary, authority, cert_path, key_path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Tag, n.Address, n.Port, string(n.Role), n.Enable, n.UUID,
		n.DNSPrimary, n.DNSSecondary, n.Authority, n.CertPath, n.KeyPath, now)
	return err
}

func (d *DB) DeleteNode(id int) error {
	res, err := d.sql.Exec(`DELETE FROM nodes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) Entrypoints() ([]netstate.Entrypoint, error) {
	out := []netstate.Entrypoint{}
	err := scan(d.sql, `SELECT id, node_id, port, remark, enable FROM entrypoints ORDER BY id`,
		func(r *sql.Rows) error {
			var e netstate.Entrypoint
			if err := r.Scan(&e.ID, &e.NodeID, &e.Port, &e.Remark, &e.Enable); err != nil {
				return err
			}
			out = append(out, e)
			return nil
		})
	return out, err
}

func (d *DB) SaveEntrypoint(e netstate.Entrypoint, now int64) (int, error) {
	if e.ID == 0 {
		res, err := d.sql.Exec(
			`INSERT INTO entrypoints (node_id, port, remark, enable, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			e.NodeID, e.Port, e.Remark, e.Enable, now)
		if err != nil {
			return 0, err
		}
		id, err := res.LastInsertId()
		return int(id), err
	}

	res, err := d.sql.Exec(
		`UPDATE entrypoints SET node_id = ?, port = ?, remark = ?, enable = ? WHERE id = ?`,
		e.NodeID, e.Port, e.Remark, e.Enable, e.ID)
	if err != nil {
		return 0, err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return 0, ErrNotFound
	}
	return e.ID, nil
}

func (d *DB) DeleteEntrypoint(id int) error {
	res, err := d.sql.Exec(`DELETE FROM entrypoints WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) Clients() ([]netstate.Client, error) {
	out := []netstate.Client{}
	err := scan(d.sql, `SELECT id, tag, uuid, COALESCE(group_id, 0), enable, expiry_at, comment, admin, device_limit, allow_exit, created_at FROM clients ORDER BY id`,
		func(r *sql.Rows) error {
			var c netstate.Client
			if err := r.Scan(&c.ID, &c.Tag, &c.UUID, &c.GroupID, &c.Enable, &c.ExpiryAt, &c.Comment, &c.Admin, &c.DeviceLimit, &c.AllowExit, &c.CreatedAt); err != nil {
				return err
			}
			out = append(out, c)
			return nil
		})
	return out, err
}

func (d *DB) SaveClient(c netstate.Client, now int64) (int, error) {
	group := any(nil)
	if c.GroupID != 0 {
		group = c.GroupID
	}

	if c.ID == 0 {
		res, err := d.sql.Exec(
			`INSERT INTO clients (tag, uuid, group_id, enable, expiry_at, comment, admin, device_limit, allow_exit, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.Tag, c.UUID, group, c.Enable, c.ExpiryAt, c.Comment, c.Admin, c.DeviceLimit, c.AllowExit, now)
		if err != nil {
			return 0, err
		}
		id, err := res.LastInsertId()
		return int(id), err
	}

	res, err := d.sql.Exec(
		`UPDATE clients SET tag = ?, uuid = CASE WHEN ? = '' THEN uuid ELSE ? END,
		        group_id = ?, enable = ?, expiry_at = ?, comment = ?, admin = ?, device_limit = ?, allow_exit = ?
		  WHERE id = ?`,
		c.Tag, c.UUID, c.UUID, group, c.Enable, c.ExpiryAt, c.Comment, c.Admin, c.DeviceLimit, c.AllowExit, c.ID)
	if err != nil {
		return 0, err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return 0, ErrNotFound
	}
	return c.ID, nil
}

func (d *DB) DeleteClient(id int) error {
	res, err := d.sql.Exec(`DELETE FROM clients WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) Groups() ([]netstate.Group, error) {
	out := []netstate.Group{}
	err := scan(d.sql, `SELECT id, tag, allow_exit, device_limit FROM groups ORDER BY id`,
		func(r *sql.Rows) error {
			var g netstate.Group
			if err := r.Scan(&g.ID, &g.Tag, &g.AllowExit, &g.DeviceLimit); err != nil {
				return err
			}
			out = append(out, g)
			return nil
		})
	if err != nil {
		return nil, err
	}

	members := map[int][]int{}
	if err := scan(d.sql, `SELECT group_id, entrypoint_id FROM group_entrypoints ORDER BY group_id, entrypoint_id`,
		func(r *sql.Rows) error {
			var group, entry int
			if err := r.Scan(&group, &entry); err != nil {
				return err
			}
			members[group] = append(members[group], entry)
			return nil
		}); err != nil {
		return nil, err
	}
	for i := range out {
		ids := members[out[i].ID]
		if ids == nil {
			ids = []int{}
		}
		out[i].EntrypointIDs = ids
	}
	return out, nil
}

func (d *DB) SaveGroup(g netstate.Group, now int64) (int, error) {
	tx, err := d.sql.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	id := g.ID
	if id == 0 {
		res, err := tx.Exec(
			`INSERT INTO groups (tag, allow_exit, device_limit, created_at) VALUES (?, ?, ?, ?)`,
			g.Tag, g.AllowExit, g.DeviceLimit, now)
		if err != nil {
			return 0, err
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return 0, err
		}
		id = int(newID)
	} else {
		res, err := tx.Exec(`UPDATE groups SET tag = ?, allow_exit = ?, device_limit = ? WHERE id = ?`, g.Tag, g.AllowExit, g.DeviceLimit, id)
		if err != nil {
			return 0, err
		}
		if affected, _ := res.RowsAffected(); affected == 0 {
			return 0, ErrNotFound
		}
	}

	if _, err := tx.Exec(`DELETE FROM group_entrypoints WHERE group_id = ?`, id); err != nil {
		return 0, err
	}
	for _, entry := range g.EntrypointIDs {
		var role string
		err := tx.QueryRow(
			`SELECT n.role FROM entrypoints e JOIN nodes n ON n.id = e.node_id WHERE e.id = ?`,
			entry).Scan(&role)
		if err == nil && role == string(netstate.RoleEgress) {
			return 0, ErrExitEntrypoint
		}
		if _, err := tx.Exec(
			`INSERT INTO group_entrypoints (group_id, entrypoint_id) VALUES (?, ?)`, id, entry); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

func (d *DB) DeleteGroup(id int) error {
	res, err := d.sql.Exec(`DELETE FROM groups WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

type NetworkSettings struct {
	RefreshMinutes   int    `json:"refreshMinutes"`
	DNSPrimary       string `json:"dnsPrimary"`
	DNSSecondary     string `json:"dnsSecondary"`
	DNSCache         int    `json:"dnsCache"`
	DNSMinTTL        int    `json:"dnsMinTtl"`
	DNSMaxTTL        int    `json:"dnsMaxTtl"`
	DNSStale         int    `json:"dnsStale"`
	MTU              int    `json:"mtu"`
	StatsSeconds     int    `json:"statsSeconds"`
	Pool             string `json:"pool"`
	BrutalMbit       int    `json:"brutalMbit"`
	MaxStreams       int    `json:"maxStreams"`
	StreamWindow     int    `json:"streamWindowKb"`
	MaxStreamWindow  int    `json:"maxStreamWindowKb"`
	ConnWindow       int    `json:"connWindowKb"`
	MaxConnWindow    int    `json:"maxConnWindowKb"`
	IdleSeconds      int    `json:"idleSeconds"`
	KeepAliveSeconds int    `json:"keepAliveSeconds"`
	SocketBuffer     int    `json:"socketBufferKb"`
}

func defaultNetworkSettings() NetworkSettings {
	return NetworkSettings{
		RefreshMinutes:   480,
		DNSPrimary:       "1.1.1.1",
		DNSSecondary:     "8.8.8.8",
		DNSCache:         4096,
		DNSMinTTL:        60,
		DNSMaxTTL:        3600,
		DNSStale:         60,
		MTU:              1500,
		StatsSeconds:     5,
		Pool:             "10.7.0.0/16",
		MaxStreams:       65536,
		StreamWindow:     2048,
		MaxStreamWindow:  6144,
		ConnWindow:       3072,
		MaxConnWindow:    15360,
		IdleSeconds:      90,
		KeepAliveSeconds: 15,
		SocketBuffer:     2048,
	}
}

func (d *DB) NetworkSettings() (NetworkSettings, error) {
	out := defaultNetworkSettings()
	err := d.sql.QueryRow(
		`SELECT refresh_minutes, dns_primary, dns_secondary, dns_cache, dns_min_ttl,
		        dns_max_ttl, dns_stale, mtu, stats_seconds, pool, brutal_mbit,
		        max_streams, stream_window, max_stream_window, conn_window, max_conn_window,
		        idle_seconds, keepalive_seconds, socket_buffer
		 FROM network WHERE id = 1`).Scan(
		&out.RefreshMinutes, &out.DNSPrimary, &out.DNSSecondary,
		&out.DNSCache, &out.DNSMinTTL, &out.DNSMaxTTL, &out.DNSStale,
		&out.MTU, &out.StatsSeconds, &out.Pool, &out.BrutalMbit, &out.MaxStreams, &out.StreamWindow, &out.MaxStreamWindow,
		&out.ConnWindow, &out.MaxConnWindow, &out.IdleSeconds, &out.KeepAliveSeconds,
		&out.SocketBuffer)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultNetworkSettings(), nil
	}
	return out.sane(), err
}

func (s NetworkSettings) sane() NetworkSettings {
	fallback := defaultNetworkSettings()

	if s.RefreshMinutes < 1 {
		s.RefreshMinutes = fallback.RefreshMinutes
	}
	if s.RefreshMinutes > 1440 {
		s.RefreshMinutes = 1440
	}
	if s.DNSPrimary == "" && s.DNSSecondary == "" {
		s.DNSPrimary, s.DNSSecondary = fallback.DNSPrimary, fallback.DNSSecondary
	}
	if s.DNSCache < 1 {
		s.DNSCache = fallback.DNSCache
	}
	if s.DNSCache > 1_000_000 {
		s.DNSCache = 1_000_000
	}
	if s.DNSMinTTL < 0 {
		s.DNSMinTTL = 0
	}
	if s.DNSMaxTTL < 1 {
		s.DNSMaxTTL = fallback.DNSMaxTTL
	}
	if s.DNSMaxTTL < s.DNSMinTTL {
		s.DNSMaxTTL = s.DNSMinTTL
	}
	if s.DNSStale < 0 {
		s.DNSStale = 0
	}
	if s.MTU < 576 || s.MTU > 9000 {
		s.MTU = fallback.MTU
	}
	if s.StatsSeconds < 0 {
		s.StatsSeconds = 0
	}
	if s.Pool == "" {
		s.Pool = fallback.Pool
	}
	if s.MaxStreams < 16 {
		s.MaxStreams = fallback.MaxStreams
	}
	if s.StreamWindow < 64 {
		s.StreamWindow = fallback.StreamWindow
	}
	if s.MaxStreamWindow < s.StreamWindow {
		s.MaxStreamWindow = s.StreamWindow
	}
	if s.ConnWindow < 64 {
		s.ConnWindow = fallback.ConnWindow
	}
	if s.MaxConnWindow < s.ConnWindow {
		s.MaxConnWindow = s.ConnWindow
	}
	if s.IdleSeconds < 5 {
		s.IdleSeconds = fallback.IdleSeconds
	}
	if s.KeepAliveSeconds < 1 || s.KeepAliveSeconds >= s.IdleSeconds {
		s.KeepAliveSeconds = s.IdleSeconds / 2
	}
	if s.SocketBuffer < 256 {
		s.SocketBuffer = fallback.SocketBuffer
	}
	if s.BrutalMbit < 0 {
		s.BrutalMbit = 0
	}
	return s
}

func (d *DB) SaveNetworkSettings(s NetworkSettings) error {
	s = s.sane()
	res, err := d.sql.Exec(
		`UPDATE network SET refresh_minutes = ?, dns_primary = ?, dns_secondary = ?,
		        dns_cache = ?, dns_min_ttl = ?, dns_max_ttl = ?, dns_stale = ?,
		        mtu = ?, stats_seconds = ?,
		        pool = ?, brutal_mbit = ?, max_streams = ?, stream_window = ?,
		        max_stream_window = ?, conn_window = ?, max_conn_window = ?,
		        idle_seconds = ?, keepalive_seconds = ?, socket_buffer = ?
		 WHERE id = 1`,
		s.RefreshMinutes, s.DNSPrimary, s.DNSSecondary,
		s.DNSCache, s.DNSMinTTL, s.DNSMaxTTL, s.DNSStale,
		s.MTU, s.StatsSeconds, s.Pool, s.BrutalMbit,
		s.MaxStreams, s.StreamWindow, s.MaxStreamWindow, s.ConnWindow, s.MaxConnWindow,
		s.IdleSeconds, s.KeepAliveSeconds, s.SocketBuffer)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

type DNSRecord struct {
	ID      int    `json:"id"`
	Suffix  string `json:"suffix"`
	V4      string `json:"v4"`
	V6      string `json:"v6"`
	Comment string `json:"comment"`
	Enable  bool   `json:"enable"`
}

func (d *DB) DNSRecords() ([]DNSRecord, error) {
	out := []DNSRecord{}
	err := scan(d.sql, `SELECT id, suffix, v4, v6, comment, enable FROM dns_records ORDER BY suffix`,
		func(r *sql.Rows) error {
			var rec DNSRecord
			if err := r.Scan(&rec.ID, &rec.Suffix, &rec.V4, &rec.V6, &rec.Comment, &rec.Enable); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	return out, err
}

var ErrEmptyRecord = errors.New("a record needs a name and at least one address")

func (d *DB) SaveDNSRecord(rec DNSRecord) (int, error) {
	rec.Suffix = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rec.Suffix), "*.")))
	rec.V4 = strings.TrimSpace(rec.V4)
	rec.V6 = strings.TrimSpace(rec.V6)
	if rec.Suffix == "" || (rec.V4 == "" && rec.V6 == "") {
		return 0, ErrEmptyRecord
	}
	if rec.V4 != "" {
		addr, err := netip.ParseAddr(rec.V4)
		if err != nil || !addr.Is4() {
			return 0, fmt.Errorf("%q is not an ipv4 address", rec.V4)
		}
	}
	if rec.V6 != "" {
		addr, err := netip.ParseAddr(rec.V6)
		if err != nil || addr.Is4() {
			return 0, fmt.Errorf("%q is not an ipv6 address", rec.V6)
		}
	}

	if rec.ID > 0 {
		res, err := d.sql.Exec(
			`UPDATE dns_records SET suffix = ?, v4 = ?, v6 = ?, comment = ?, enable = ? WHERE id = ?`,
			rec.Suffix, rec.V4, rec.V6, rec.Comment, rec.Enable, rec.ID)
		if err != nil {
			return 0, err
		}
		if affected, _ := res.RowsAffected(); affected == 0 {
			return 0, ErrNotFound
		}
		return rec.ID, nil
	}

	res, err := d.sql.Exec(
		`INSERT INTO dns_records (suffix, v4, v6, comment, enable) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(suffix) DO UPDATE SET v4 = excluded.v4, v6 = excluded.v6,
		        comment = excluded.comment, enable = excluded.enable`,
		rec.Suffix, rec.V4, rec.V6, rec.Comment, rec.Enable)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil || id == 0 {
		err = d.sql.QueryRow(`SELECT id FROM dns_records WHERE suffix = ?`, rec.Suffix).Scan(&rec.ID)
		return rec.ID, err
	}
	return int(id), nil
}

func (d *DB) DeleteDNSRecord(id int) error {
	res, err := d.sql.Exec(`DELETE FROM dns_records WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) settle(held, want netstate.Node, now int64) (netstate.Node, error) {
	before := held
	if held.UUID == "" {
		held.UUID = want.UUID
	}
	if want.Authority != "" {
		held.Authority = want.Authority
	}
	if want.CertPath != "" {
		held.CertPath = want.CertPath
	}
	if want.KeyPath != "" {
		held.KeyPath = want.KeyPath
	}
	if held == before {
		return held, nil
	}
	if _, err := d.SaveNode(held, now); err != nil {
		return netstate.Node{}, err
	}
	return held, nil
}
