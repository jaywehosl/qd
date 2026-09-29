package clientstate

import (
	"time"

	"github.com/quic-go/quic-go"
)

type tokens struct{ db *DB }

func (d *DB) Tokens() quic.TokenStore { return tokens{db: d} }

func (t tokens) Pop(server string) *quic.ClientToken {
	var blob []byte
	var rtt int64
	err := t.db.sql.QueryRow(`
		DELETE FROM tokens WHERE server = ? AND stored > ?
		RETURNING blob, rtt`, server, time.Now().Add(-tokenAge).Unix()).Scan(&blob, &rtt)
	if err != nil {
		return nil
	}
	return quic.NewClientToken(blob, time.Duration(rtt))
}

func (t tokens) Put(server string, token *quic.ClientToken) {
	t.db.sql.Exec(`
		INSERT INTO tokens (server, blob, rtt, stored) VALUES (?, ?, ?, ?)
		ON CONFLICT(server) DO UPDATE SET blob = excluded.blob, rtt = excluded.rtt, stored = excluded.stored`,
		server, token.Data(), int64(token.RTT()), time.Now().Unix())
}

const tokenAge = 24 * time.Hour
