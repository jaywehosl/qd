package clientstate

import (
	"crypto/tls"
	"encoding/binary"
	"time"
)

type tickets struct{ db *DB }

func (d *DB) Tickets() tls.ClientSessionCache { return tickets{db: d} }

func (t tickets) Get(server string) (*tls.ClientSessionState, bool) {
	var blob []byte
	if err := t.db.sql.QueryRow(`SELECT blob FROM tickets WHERE server = ?`, server).Scan(&blob); err != nil {
		return nil, false
	}
	if len(blob) < 4 {
		return nil, false
	}
	size := binary.BigEndian.Uint32(blob)
	if uint64(size) > uint64(len(blob)-4) {
		return nil, false
	}
	state, err := tls.ParseSessionState(blob[4+size:])
	if err != nil {
		return nil, false
	}
	held, err := tls.NewResumptionState(blob[4:4+size], state)
	if err != nil {
		return nil, false
	}
	return held, true
}

func (t tickets) Put(server string, held *tls.ClientSessionState) {
	if held == nil {
		t.db.sql.Exec(`DELETE FROM tickets WHERE server = ?`, server)
		return
	}
	ticket, state, err := held.ResumptionState()
	if err != nil || state == nil {
		return
	}
	raw, err := state.Bytes()
	if err != nil {
		return
	}
	blob := make([]byte, 4, 4+len(ticket)+len(raw))
	binary.BigEndian.PutUint32(blob, uint32(len(ticket)))
	blob = append(blob, ticket...)
	blob = append(blob, raw...)
	t.db.sql.Exec(`
		INSERT INTO tickets (server, blob, stored) VALUES (?, ?, ?)
		ON CONFLICT(server) DO UPDATE SET blob = excluded.blob, stored = excluded.stored`,
		server, blob, time.Now().Unix())
}
