//go:build windows

package main

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
)

func collectSamples(db *clientstate.DB, tun *tunnel, stop <-chan struct{}) {
	t := time.NewTicker(time.Second)
	defer t.Stop()

	var prev counters
	var prevDNS dnsSnapshot

	last := time.Now()
	var spoken time.Time
	var window, windowIn, windowOut uint64

	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}

		stamp := time.Now()
		now := stamp.Unix()
		cur := snapshotStats()
		dns := snapshotDNS(tun.DNS())

		// Тик приходит не ровно через секунду — в фоне система будит процесс реже.
		// Делить прибавку на «секунду» значит занижать скорость ровно на опоздание:
		// отсюда и пила на графике, и разница между окном в фокусе и свёрнутым.
		span := stamp.Sub(last)
		last = stamp
		if span <= 0 {
			span = time.Second
		}
		per := float64(time.Second) / float64(span)

		sample := clientstate.Sample{
			T:           now,
			Up:          rate(delta(cur.bytesOut, prev.bytesOut), per),
			Down:        rate(delta(cur.bytesIn, prev.bytesIn), per),
			PktOut:      rate(delta(cur.out, prev.out), per),
			PktIn:       rate(delta(cur.in, prev.in), per),
			Lost:        delta(cur.gaps, prev.gaps),
			Reorder:     delta(cur.reorder, prev.reorder),
			Retries:     delta(cur.retries, prev.retries),
			SendDrop:    delta(cur.sendDrop, prev.sendDrop),
			SendErr:     delta(cur.sendErr, prev.sendErr),
			DNSQueries:  delta(uint64(dns.queries), uint64(prevDNS.queries)),
			DNSCached:   delta(uint64(dns.cached), uint64(prevDNS.cached)),
			DNSUpstream: delta(uint64(dns.upstream), uint64(prevDNS.upstream)),
			Adblock:     delta(uint64(dns.blocked), uint64(prevDNS.blocked)),
		}
		sentRaw := delta(cur.bytesOut, prev.bytesOut)
		gotRaw := delta(cur.bytesIn, prev.bytesIn)
		prev, prevDNS = cur, dns

		if sentRaw > 0 || gotRaw > 0 {
			db.AddTraffic(sentRaw, gotRaw)
		}
		db.AddSample(sample)

		windowOut += uint64(sentRaw)
		windowIn += uint64(gotRaw)
		window += uint64(span)
		if spoken.IsZero() {
			spoken = stamp
		}
		if stamp.Sub(spoken) >= speedEvery {
			say := time.Duration(window).Seconds()
			if say > 0 && (windowIn > 0 || windowOut > 0) {
				fmt.Printf("speed    %.1f Mbit down, %.1f Mbit up over the last %.0fs\n",
					float64(windowIn)*8/say/1e6, float64(windowOut)*8/say/1e6, say)
			}
			spoken, window, windowIn, windowOut = stamp, 0, 0, 0
		}
	}
}

type dnsSnapshot struct {
	queries  int64
	cached   int64
	upstream int64
	blocked  int64
}

type counters struct {
	out, in, bytesOut, bytesIn uint64
	gaps, reorder, retries     uint64
	sendDrop, sendErr          uint64
}

func snapshotStats() counters {
	live := liveTunnel.Load()
	if live == nil {
		return counters{}
	}
	c := (*live).Stats()
	return counters{
		out:      c.Out,
		in:       c.In,
		bytesOut: c.BytesOut,
		bytesIn:  c.BytesIn,
	}
}

func snapshotDNS(r *resolver) dnsSnapshot {
	if r == nil {
		return dnsSnapshot{}
	}
	return dnsSnapshot{
		queries:  int64(r.stats.queries.Load()),
		cached:   int64(r.stats.hits.Load()),
		upstream: int64(r.stats.upstream.Load()),
		blocked:  int64(r.stats.blocked.Load()),
	}
}

func delta(now, before uint64) int64 {
	if now < before {
		return 0
	}
	return int64(now - before)
}

// connectNow поднимает туннель гонкой по всем точкам входа сразу: кандидата не
// выбираем заранее, побеждает тот, кто первым отдал адрес.
func connectNow(db *clientstate.DB, tun *tunnel, sub clientstate.Subscription, key *qdcrypt.Key) error {
	nodes, err := db.Nodes()
	if err != nil {
		return err
	}
	session := clientstate.SessionID(sub.Key)

	lane := entrypointsOf(nodes)
	if len(lane) == 0 {
		return fmt.Errorf("no entrypoint to dial")
	}
	if err := tun.Start(lane, session); err != nil {
		return err
	}

	db.ClearSelection()
	won := tun.ServerName()
	for _, n := range nodes {
		if net.JoinHostPort(n.Address, strconv.Itoa(n.Port)) != won {
			continue
		}
		db.MarkNode(n.ID, n.LatencyMs, true, true)
		db.Notify("info", "Connected through "+n.Name+".", time.Now().UnixMilli())
		break
	}
	return nil
}

// entrypointsOf — все входы подписки. Выходные узлы клиент не набирает: до них
// добирается ingress, и делает это своей гонкой.
func entrypointsOf(nodes []clientstate.Node) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n.Role == "egress" || n.Address == "" || n.Port == 0 {
			continue
		}
		out = append(out, net.JoinHostPort(n.Address, strconv.Itoa(n.Port)))
	}
	return out
}

func rate(n int64, per float64) int64 {
	if n <= 0 {
		return 0
	}
	return int64(float64(n)*per + 0.5)
}

const speedEvery = 10 * time.Second
