package clientstate

func (d *DB) ECH(server string) []byte {
	var list []byte
	d.sql.QueryRow(`SELECT list FROM ech WHERE server = ?`, server).Scan(&list)
	return list
}

func (d *DB) PutECH(server string, list []byte) {
	if len(list) == 0 {
		d.sql.Exec(`DELETE FROM ech WHERE server = ?`, server)
		return
	}
	d.sql.Exec(`
		INSERT INTO ech (server, list) VALUES (?, ?)
		ON CONFLICT(server) DO UPDATE SET list = excluded.list`, server, list)
}
