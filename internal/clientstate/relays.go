package clientstate

import (
	"errors"
	"strings"
	"time"
)

type Relay struct {
	ID      int    `json:"id"`
	Link    string `json:"link"`
	Enabled bool   `json:"enabled"`
}

var ErrRelayHeld = errors.New("this relay is already in the list")

func (d *DB) Relays() ([]Relay, error) {
	rows, err := d.sql.Query(`SELECT id, link, enabled FROM relays ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Relay{}
	for rows.Next() {
		var r Relay
		if err := rows.Scan(&r.ID, &r.Link, &r.Enabled); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) AddRelay(link string) (int, error) {
	res, err := d.sql.Exec(`INSERT INTO relays (link, enabled, created) VALUES (?, 1, ?)`, link, time.Now().UnixMilli())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, ErrRelayHeld
		}
		return 0, err
	}
	id, err := res.LastInsertId()
	return int(id), err
}

func (d *DB) EnableRelay(id int, on bool) error {
	_, err := d.sql.Exec(`UPDATE relays SET enabled = ? WHERE id = ?`, on, id)
	return err
}

func (d *DB) DeleteRelay(id int) error {
	_, err := d.sql.Exec(`DELETE FROM relays WHERE id = ?`, id)
	return err
}
