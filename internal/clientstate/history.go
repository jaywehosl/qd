package clientstate

type Sample struct {
	T           int64 `json:"t"`
	Up          int64 `json:"up"`
	Down        int64 `json:"down"`
	PktOut      int64 `json:"pktOut"`
	PktIn       int64 `json:"pktIn"`
	Lost        int64 `json:"lost"`
	Drops       int64 `json:"drops"`
	Reorder     int64 `json:"reorder"`
	Retries     int64 `json:"retries"`
	SendDrop    int64 `json:"sendDrop"`
	SendErr     int64 `json:"sendErr"`
	DNSQueries  int64 `json:"dnsQueries"`
	DNSCached   int64 `json:"dnsCached"`
	DNSUpstream int64 `json:"dnsUpstream"`
	Adblock     int64 `json:"adblock"`
}

const sampleCap = 3900

func (d *DB) AddSample(s Sample) error {
	if _, err := d.sql.Exec(`
		INSERT OR REPLACE INTO samples
			(t, up, down, pkt_out, pkt_in, lost, drops, reorder, retries,
			 send_drop, send_err, dns_queries, dns_cached, dns_upstream, adblock)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.T, s.Up, s.Down, s.PktOut, s.PktIn, s.Lost, s.Drops, s.Reorder, s.Retries,
		s.SendDrop, s.SendErr, s.DNSQueries, s.DNSCached, s.DNSUpstream, s.Adblock); err != nil {
		return err
	}
	_, err := d.sql.Exec(
		`DELETE FROM samples WHERE t NOT IN (SELECT t FROM samples ORDER BY t DESC LIMIT ?)`,
		sampleCap)
	return err
}

func (d *DB) Samples(since, until int64, points int) ([]Sample, error) {
	rows, err := d.sql.Query(`
		SELECT t, up, down, pkt_out, pkt_in, lost, drops, reorder, retries,
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
			&s.Reorder, &s.Retries, &s.SendDrop, &s.SendErr,
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
	if width < 1 {
		width = 1
	}
	count := int((span + width - 1) / width)
	if count < 1 {
		count = 1
	}

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
	dst.Reorder = max(dst.Reorder, s.Reorder)
	dst.Retries = max(dst.Retries, s.Retries)
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

type Site struct {
	Host string `json:"host"`
	Hits int64  `json:"hits"`
}

func (d *DB) NoteSite(host string) error {
	_, err := d.sql.Exec(`
		INSERT INTO sites (host, hits) VALUES (?, 1)
		ON CONFLICT(host) DO UPDATE SET hits = sites.hits + 1`, host)
	return err
}

func (d *DB) TopSites(limit int) ([]Site, error) {
	rows, err := d.sql.Query(`SELECT host, hits FROM sites ORDER BY hits DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Site{}
	for rows.Next() {
		var s Site
		if err := rows.Scan(&s.Host, &s.Hits); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
