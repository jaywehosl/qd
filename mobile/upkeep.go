package qdmobile

import "time"

func (c *Client) upkeep() {
	go c.meter()
	go c.api.KeepFresh(c.quit)
	go c.api.KeepProbing(c.quit, 60*time.Second)
	go c.api.Greet()
}

func (c *Client) meter() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	var prevIn, prevOut uint64

	for {
		select {
		case <-c.quit:
			return
		case <-tick.C:
		}

		c.mu.Lock()
		live := c.live
		c.mu.Unlock()
		if live == nil {
			prevIn, prevOut = 0, 0
			continue
		}

		got := live.Stats()
		if got.BytesIn >= prevIn && got.BytesOut >= prevOut {
			up := int64(got.BytesOut - prevOut)
			down := int64(got.BytesIn - prevIn)
			if up > 0 || down > 0 {
				c.db.AddTraffic(up, down)
			}
		}
		prevIn, prevOut = got.BytesIn, got.BytesOut
	}
}
