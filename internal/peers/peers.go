package peers

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"
)

const wait = 3 * time.Second

var resolved sync.Map

func Prefixes(hosts []string, fail func(host string, err error)) []netip.Prefix {
	found := make([][]netip.Addr, len(hosts))
	var wg sync.WaitGroup
	for i, host := range hosts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			addrs, err := lookUp(host)
			if err != nil && fail != nil {
				fail(host, err)
			}
			found[i] = addrs
		}()
	}
	wg.Wait()

	out := []netip.Prefix{}
	for _, addrs := range found {
		for _, addr := range addrs {
			out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
		}
	}
	return out
}

func lookUp(host string) ([]netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{addr.Unmap()}, nil
	}
	if held, known := resolved.Load(host); known {
		go refresh(host)
		return held.([]netip.Addr), nil
	}
	return refresh(host)
}

func refresh(host string) ([]netip.Addr, error) {
	ctx, stop := context.WithTimeout(context.Background(), wait)
	defer stop()

	found, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		if held, known := resolved.Load(host); known {
			return held.([]netip.Addr), nil
		}
		return nil, err
	}

	out := make([]netip.Addr, 0, len(found))
	for _, addr := range found {
		out = append(out, addr.Unmap())
	}
	resolved.Store(host, out)
	return out, nil
}
