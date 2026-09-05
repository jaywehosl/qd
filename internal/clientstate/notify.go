package clientstate

type Notification struct {
	ID       int    `json:"id"`
	Severity string `json:"severity"`
	Text     string `json:"text"`
	TS       int64  `json:"ts"`
	Read     bool   `json:"read"`
}

const notificationCap = 200

func (d *DB) Notify(severity, text string, now int64) error {
	if _, err := d.sql.Exec(
		`INSERT INTO notifications (severity, text, ts, read) VALUES (?, ?, ?, 0)`,
		severity, text, now); err != nil {
		return err
	}
	_, err := d.sql.Exec(
		`DELETE FROM notifications WHERE id NOT IN (
			SELECT id FROM notifications ORDER BY id DESC LIMIT ?)`, notificationCap)
	return err
}

func (d *DB) Notifications() ([]Notification, int, error) {
	rows, err := d.sql.Query(
		`SELECT id, severity, text, ts, read FROM notifications ORDER BY id DESC`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []Notification{}
	unread := 0
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Severity, &n.Text, &n.TS, &n.Read); err != nil {
			return nil, 0, err
		}
		if !n.Read {
			unread++
		}
		out = append(out, n)
	}
	return out, unread, rows.Err()
}

func (d *DB) MarkRead(id int) error {
	if id == 0 {
		_, err := d.sql.Exec(`UPDATE notifications SET read = 1`)
		return err
	}
	_, err := d.sql.Exec(`UPDATE notifications SET read = 1 WHERE id = ?`, id)
	return err
}

func (d *DB) DismissNotification(id int) error {
	_, err := d.sql.Exec(`DELETE FROM notifications WHERE id = ?`, id)
	return err
}

func (d *DB) ClearNotifications() error {
	_, err := d.sql.Exec(`DELETE FROM notifications`)
	return err
}
