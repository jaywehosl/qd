package store

func (d *DB) Version() (int, error) {
	var n int
	err := d.sql.QueryRow(`SELECT COALESCE(MAX(number), 0) FROM revisions`).Scan(&n)
	return n, err
}

func (d *DB) Bump(now int64) (int, error) {
	if _, err := d.sql.Exec(
		`INSERT INTO revisions (number, created_at)
		 SELECT COALESCE(MAX(number), 0) + 1, ? FROM revisions`, now); err != nil {
		return 0, err
	}
	return d.Version()
}

func (d *DB) Settle(number int, now int64) (int, error) {
	if number <= 0 {
		return d.Bump(now)
	}
	if _, err := d.sql.Exec(
		`INSERT INTO revisions (number, created_at) VALUES (?, ?) ON CONFLICT(number) DO NOTHING`,
		number, now); err != nil {
		return 0, err
	}
	return d.Version()
}
