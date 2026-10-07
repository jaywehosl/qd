package qdmobile

import (
	"time"

	"github.com/jaywehosl/qd/internal/clientdns"
	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/qcli"
)

func (c *Client) upkeep() {
	go c.meter()
	go c.api.KeepFresh(c.quit)
	go c.api.KeepProbing(c.quit, 60*time.Second)
	go c.api.Greet()
}

func (c *Client) meter() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	var prev qcli.Counters
	var prevDNS clientdns.Stats
	quiet := 0

	for {
		select {
		case <-c.quit:
			return
		case <-tick.C:
		}

		c.mu.Lock()
		live, warm := c.live, c.dns
		c.mu.Unlock()
		if live == nil {
			prev, prevDNS = qcli.Counters{}, clientdns.Stats{}
			continue
		}

		got := live.Stats()
		var dns clientdns.Stats
		if warm != nil {
			dns = warm.Stats()
		}

		up, down := gap(got.BytesOut, prev.BytesOut), gap(got.BytesIn, prev.BytesIn)
		if up > 0 || down > 0 {
			c.db.AddTraffic(up, down)
		}
		if got == prev && dns == prevDNS {
			quiet++
			if quiet%quietStep != 0 {
				continue
			}
		} else {
			quiet = 0
		}
		c.db.AddSample(clientstate.Sample{
			T:           time.Now().Unix(),
			Up:          up,
			Down:        down,
			PktOut:      gap(got.Out, prev.Out),
			PktIn:       gap(got.In, prev.In),
			DNSQueries:  gap(dns.Queries, prevDNS.Queries),
			DNSCached:   gap(dns.Hits, prevDNS.Hits),
			DNSUpstream: gap(dns.Upstream, prevDNS.Upstream),
			Adblock:     gap(dns.Blocked, prevDNS.Blocked),
		})
		prev, prevDNS = got, dns
	}
}

const quietStep = 30

func gap(now, before uint64) int64 {
	if now < before {
		return 0
	}
	return int64(now - before)
}
