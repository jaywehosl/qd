package clientstate

type Sample struct {
	T           int64 `json:"t"`
	Up          int64 `json:"up"`
	Down        int64 `json:"down"`
	PktOut      int64 `json:"pktOut"`
	PktIn       int64 `json:"pktIn"`
	Lost        int64 `json:"lost"`
	Drops       int64 `json:"drops"`
	SendDrop    int64 `json:"sendDrop"`
	SendErr     int64 `json:"sendErr"`
	DNSQueries  int64 `json:"dnsQueries"`
	DNSCached   int64 `json:"dnsCached"`
	DNSUpstream int64 `json:"dnsUpstream"`
	Adblock     int64 `json:"adblock"`
}

const sampleKeep = 3900

func (s Sample) Quiet() bool {
	return s.Up == 0 && s.Down == 0 && s.PktOut == 0 && s.PktIn == 0 && s.Lost == 0 && s.Drops == 0 &&
		s.SendDrop == 0 && s.SendErr == 0 && s.DNSQueries == 0 && s.DNSCached == 0 && s.DNSUpstream == 0 && s.Adblock == 0
}

func (d *DB) AddSample(s Sample) error {
	if _, err := d.sql.Exec(`
		INSERT OR REPLACE INTO samples
			(t, up, down, pkt_out, pkt_in, lost, drops,
			 send_drop, send_err, dns_queries, dns_cached, dns_upstream, adblock)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.T, s.Up, s.Down, s.PktOut, s.PktIn, s.Lost, s.Drops,
		s.SendDrop, s.SendErr, s.DNSQueries, s.DNSCached, s.DNSUpstream, s.Adblock); err != nil {
		return err
	}
	if s.T%60 != 0 {
		return nil
	}
	_, err := d.sql.Exec(`DELETE FROM samples WHERE t < ?`, s.T-sampleKeep)
	return err
}

func (d *DB) Samples(since, until int64, points int) ([]Sample, error) {
	rows, err := d.sql.Query(`
		SELECT t, up, down, pkt_out, pkt_in, lost, drops,
		       send_drop, send_err, dns_queries, dns_cached, dns_upstream, adblock
		  FROM samples WHERE t >= ? ORDER BY t`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	raw := []Sample{}
	for rows.Next() {
		var s Sample
		if err := rows.Scan(&s.T, &s.Up, &s.Down, &s.PktOut, &s.PktIn, &s.Lost, &s.Drops,
			&s.SendDrop, &s.SendErr,
			&s.DNSQueries, &s.DNSCached, &s.DNSUpstream, &s.Adblock); err != nil {
			return nil, err
		}
		raw = append(raw, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if points < 1 {
		points = 1
	}
	return bucketByTime(raw, since, until, points), nil
}

func bucketByTime(raw []Sample, since, until int64, points int) []Sample {
	if len(raw) == 0 {
		return []Sample{}
	}
	if raw[0].T > since {
		since = raw[0].T
	}

	span := until - since
	if span < 1 {
		span = 1
	}
	width := (span + int64(points) - 1) / int64(points)
	count := int((span + width - 1) / width)

	out := make([]Sample, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, Sample{T: since + int64(i)*width})
	}

	for _, s := range raw {
		i := int((s.T - since) / width)
		if i < 0 || i >= len(out) {
			continue
		}
		peak(&out[i], s)
	}
	return out
}

func peak(dst *Sample, s Sample) {
	dst.Up = max(dst.Up, s.Up)
	dst.Down = max(dst.Down, s.Down)
	dst.PktOut = max(dst.PktOut, s.PktOut)
	dst.PktIn = max(dst.PktIn, s.PktIn)
	dst.Lost = max(dst.Lost, s.Lost)
	dst.Drops = max(dst.Drops, s.Drops)
	dst.SendDrop = max(dst.SendDrop, s.SendDrop)
	dst.SendErr = max(dst.SendErr, s.SendErr)
	dst.DNSQueries = max(dst.DNSQueries, s.DNSQueries)
	dst.DNSCached = max(dst.DNSCached, s.DNSCached)
	dst.DNSUpstream = max(dst.DNSUpstream, s.DNSUpstream)
	dst.Adblock = max(dst.Adblock, s.Adblock)
}

func (d *DB) AddTraffic(up, down int64) error {
	_, err := d.sql.Exec(`
		INSERT INTO traffic (id, up, down) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET up = traffic.up + ?, down = traffic.down + ?`,
		up, down, up, down)
	return err
}

func (d *DB) Traffic() (up, down int64, err error) {
	err = d.sql.QueryRow(`SELECT up, down FROM traffic WHERE id = 1`).Scan(&up, &down)
	if err != nil {
		return 0, 0, nil
	}
	return up, down, nil
}
