package store

import "database/sql"

type Reading struct {
	ClientID int
	NodeID   int
	Epoch    int64
	Up       uint64
	Down     uint64
	At       int64
}

func (d *DB) RecordTraffic(readings []Reading) error {
	if len(readings) == 0 {
		return nil
	}

	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, r := range readings {
		var epoch int64
		var lastUp, lastDown uint64
		err := tx.QueryRow(
			`SELECT epoch, last_up, last_down FROM client_traffic WHERE client_id = ? AND node_id = ?`,
			r.ClientID, r.NodeID).Scan(&epoch, &lastUp, &lastDown)

		switch {
		case err == sql.ErrNoRows:
			if _, err := tx.Exec(
				`INSERT INTO client_traffic (client_id, node_id, epoch, last_up, last_down, up, down, at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				r.ClientID, r.NodeID, r.Epoch, r.Up, r.Down, r.Up, r.Down, r.At); err != nil {
				return err
			}
			continue
		case err != nil:
			return err
		}

		addUp, addDown := r.Up, r.Down
		if epoch == r.Epoch {
			if r.Up >= lastUp {
				addUp = r.Up - lastUp
			}
			if r.Down >= lastDown {
				addDown = r.Down - lastDown
			}
		}

		if _, err := tx.Exec(
			`UPDATE client_traffic
			    SET epoch = ?, last_up = ?, last_down = ?,
			        up = up + ?, down = down + ?,
			        at = CASE WHEN ? > 0 THEN ? ELSE at END
			  WHERE client_id = ? AND node_id = ?`,
			r.Epoch, r.Up, r.Down, addUp, addDown,
			addUp+addDown, r.At, r.ClientID, r.NodeID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) RecordPeerTraffic(readings []Reading) error {
	if len(readings) == 0 {
		return nil
	}

	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, r := range readings {
		var epoch int64
		var lastUp, lastDown uint64
		err := tx.QueryRow(
			`SELECT epoch, last_up, last_down FROM peer_traffic WHERE peer_id = ? AND node_id = ?`,
			r.ClientID, r.NodeID).Scan(&epoch, &lastUp, &lastDown)

		switch {
		case err == sql.ErrNoRows:
			if _, err := tx.Exec(
				`INSERT INTO peer_traffic (peer_id, node_id, epoch, last_up, last_down, up, down, at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				r.ClientID, r.NodeID, r.Epoch, r.Up, r.Down, r.Up, r.Down, r.At); err != nil {
				return err
			}
			continue
		case err != nil:
			return err
		}

		addUp, addDown := r.Up, r.Down
		if epoch == r.Epoch {
			if r.Up >= lastUp {
				addUp = r.Up - lastUp
			}
			if r.Down >= lastDown {
				addDown = r.Down - lastDown
			}
		}

		if _, err := tx.Exec(
			`UPDATE peer_traffic
			    SET epoch = ?, last_up = ?, last_down = ?,
			        up = up + ?, down = down + ?,
			        at = CASE WHEN ? > 0 THEN ? ELSE at END
			  WHERE peer_id = ? AND node_id = ?`,
			r.Epoch, r.Up, r.Down, addUp, addDown,
			addUp+addDown, r.At, r.ClientID, r.NodeID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) PeerTraffic() (map[int]Totals, error) {
	out := map[int]Totals{}
	err := scan(d.sql, `SELECT node_id, SUM(up), SUM(down), MAX(at) FROM peer_traffic GROUP BY node_id`,
		func(r *sql.Rows) error {
			var id int
			var t Totals
			if err := r.Scan(&id, &t.Up, &t.Down, &t.At); err != nil {
				return err
			}
			out[id] = t
			return nil
		})
	return out, err
}

func (d *DB) ResetTraffic(clientID int) error {
	_, err := d.sql.Exec(
		`UPDATE client_traffic SET up = 0, down = 0, last_up = 0, last_down = 0, epoch = 0
		  WHERE client_id = ?`, clientID)
	return err
}

type Totals struct {
	Up   uint64 `json:"up"`
	Down uint64 `json:"down"`
	At   int64  `json:"at"`
}

func (d *DB) TrafficFrom(nodeID int) (map[int]Totals, error) {
	out := map[int]Totals{}
	err := scan(d.sql, `SELECT client_id, up, down, at FROM client_traffic WHERE node_id = ?`,
		func(r *sql.Rows) error {
			var id int
			var t Totals
			if err := r.Scan(&id, &t.Up, &t.Down, &t.At); err != nil {
				return err
			}
			out[id] = t
			return nil
		}, nodeID)
	return out, err
}

func (d *DB) Traffic() (map[int]Totals, error) {
	out := map[int]Totals{}
	err := scan(d.sql, `SELECT client_id, SUM(up), SUM(down), MAX(at) FROM client_traffic GROUP BY client_id`,
		func(r *sql.Rows) error {
			var id int
			var t Totals
			if err := r.Scan(&id, &t.Up, &t.Down, &t.At); err != nil {
				return err
			}
			out[id] = t
			return nil
		})
	return out, err
}

type Device struct {
	Fingerprint string `json:"fingerprint"`
	Platform    string `json:"platform"`
	Model       string `json:"model"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Blocked     bool   `json:"blocked"`
	NodeID      int    `json:"nodeId"`
	FirstSeen   int64  `json:"firstSeen"`
	LastSeen    int64  `json:"lastSeen"`
	Up          uint64 `json:"up"`
	Down        uint64 `json:"down"`
}

func (d *DB) RecordDevice(clientID, nodeID int, dev Device, at int64) error {
	if dev.Fingerprint == "" {
		return nil
	}
	kind := dev.Kind
	if kind == "" {
		kind = "desktop"
	}
	_, err := d.sql.Exec(`
		INSERT INTO devices (client_id, fingerprint, platform, model, kind, node_id, first_seen, last_seen, up, down)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 0)
		ON CONFLICT(client_id, fingerprint) DO UPDATE SET
			platform  = excluded.platform,
			model     = excluded.model,
			kind      = excluded.kind,
			node_id   = excluded.node_id,
			last_seen = MAX(devices.last_seen, excluded.last_seen)`,
		clientID, dev.Fingerprint, dev.Platform, labelled(dev), kind, nodeID, at, at)
	return err
}

func labelled(dev Device) string {
	if dev.Name == "" {
		return dev.Model
	}
	if dev.Model == "" {
		return dev.Name
	}
	return dev.Model + " · " + dev.Name
}

func (d *DB) Device(clientID int, fingerprint string) (Device, bool, error) {
	var dev Device
	err := d.sql.QueryRow(
		`SELECT fingerprint, platform, model, kind, blocked, node_id, first_seen, last_seen, up, down
		   FROM devices WHERE client_id = ? AND fingerprint = ?`, clientID, fingerprint).
		Scan(&dev.Fingerprint, &dev.Platform, &dev.Model, &dev.Kind, &dev.Blocked,
			&dev.NodeID, &dev.FirstSeen, &dev.LastSeen, &dev.Up, &dev.Down)
	if err == sql.ErrNoRows {
		return Device{}, false, nil
	}
	return dev, err == nil, err
}

func (d *DB) CountDevices(clientID int) (int, error) {
	var n int
	err := d.sql.QueryRow(
		`SELECT COUNT(*) FROM devices WHERE client_id = ? AND blocked = 0`, clientID).Scan(&n)
	return n, err
}

func (d *DB) BlockDevice(clientID int, fingerprint string, blocked bool) error {
	res, err := d.sql.Exec(
		`UPDATE devices SET blocked = ? WHERE client_id = ? AND fingerprint = ?`,
		blocked, clientID, fingerprint)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) ForgetDevice(clientID int, fingerprint string) (int, error) {
	res, err := d.sql.Exec(
		`DELETE FROM devices WHERE client_id = ? AND fingerprint = ?`, clientID, fingerprint)
	if err != nil {
		return 0, err
	}
	gone, _ := res.RowsAffected()
	return int(gone), nil
}

func (d *DB) Devices() (map[int][]Device, error) {
	out := map[int][]Device{}
	err := scan(d.sql,
		`SELECT client_id, fingerprint, platform, model, kind, blocked, node_id, first_seen, last_seen, up, down
		   FROM devices ORDER BY last_seen DESC`,
		func(r *sql.Rows) error {
			var id int
			var dev Device
			if err := r.Scan(&id, &dev.Fingerprint, &dev.Platform, &dev.Model, &dev.Kind,
				&dev.Blocked, &dev.NodeID, &dev.FirstSeen, &dev.LastSeen, &dev.Up, &dev.Down); err != nil {
				return err
			}
			out[id] = append(out[id], dev)
			return nil
		})
	return out, err
}

type Address struct {
	IP          string `json:"ip"`
	Fingerprint string `json:"fingerprint"`
	NodeID      int    `json:"nodeId"`
	LastSeen    int64  `json:"lastOnline"`
}

func (d *DB) RecordAddresses(clientID, nodeID int, fingerprint string, seen map[string]int64) error {
	if len(seen) == 0 {
		return nil
	}

	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for ip, at := range seen {
		if _, err := tx.Exec(`
			INSERT INTO ip_log (client_id, fingerprint, ip, node_id, last_seen)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(client_id, fingerprint, ip)
			DO UPDATE SET last_seen = MAX(ip_log.last_seen, excluded.last_seen), node_id = excluded.node_id`,
			clientID, fingerprint, ip, nodeID, at); err != nil {
			return err
		}

		if fingerprint != "" {
			tx.Exec(`DELETE FROM ip_log WHERE client_id = ? AND ip = ? AND fingerprint = ''`, clientID, ip)
		}
	}
	return tx.Commit()
}

func (d *DB) Addresses() (map[int][]Address, error) {
	out := map[int][]Address{}
	err := scan(d.sql, `SELECT client_id, ip, fingerprint, node_id, last_seen FROM ip_log ORDER BY last_seen DESC`,
		func(r *sql.Rows) error {
			var id int
			var a Address
			if err := r.Scan(&id, &a.IP, &a.Fingerprint, &a.NodeID, &a.LastSeen); err != nil {
				return err
			}
			out[id] = append(out[id], a)
			return nil
		})
	return out, err
}

func (d *DB) ForgetAddress(clientID int, ip string) (int, error) {
	res, err := d.sql.Exec(`DELETE FROM ip_log WHERE client_id = ? AND ip = ?`, clientID, ip)
	if err != nil {
		return 0, err
	}
	gone, _ := res.RowsAffected()
	return int(gone), nil
}

func (d *DB) ForgetAddresses(clientID int) (int, error) {
	res, err := d.sql.Exec(`DELETE FROM ip_log WHERE client_id = ?`, clientID)
	if err != nil {
		return 0, err
	}
	gone, _ := res.RowsAffected()
	return int(gone), nil
}

type Exit struct {
	NodeID    int    `json:"nodeId"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastOnline"`
	Up        uint64 `json:"up"`
	Down      uint64 `json:"down"`
}

func (d *DB) RecordExit(clientID, nodeID int, at int64, up, down uint64) error {
	_, err := d.sql.Exec(`
		INSERT INTO exit_log (client_id, node_id, first_seen, last_seen, up, down)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(client_id, node_id) DO UPDATE SET
			last_seen = MAX(exit_log.last_seen, excluded.last_seen),
			up        = MAX(exit_log.up, excluded.up),
			down      = MAX(exit_log.down, excluded.down)`,
		clientID, nodeID, at, at, up, down)
	return err
}

func (d *DB) Exits() (map[int][]Exit, error) {
	out := map[int][]Exit{}
	err := scan(d.sql, `SELECT client_id, node_id, first_seen, last_seen, up, down FROM exit_log ORDER BY last_seen DESC`,
		func(r *sql.Rows) error {
			var id int
			var e Exit
			if err := r.Scan(&id, &e.NodeID, &e.FirstSeen, &e.LastSeen, &e.Up, &e.Down); err != nil {
				return err
			}
			out[id] = append(out[id], e)
			return nil
		})
	return out, err
}

func (d *DB) ForgetExit(clientID, nodeID int) (int, error) {
	query, args := `DELETE FROM exit_log WHERE client_id = ?`, []any{clientID}
	if nodeID > 0 {
		query += ` AND node_id = ?`
		args = append(args, nodeID)
	}
	res, err := d.sql.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	gone, _ := res.RowsAffected()
	return int(gone), nil
}
