package panel

import (
	"sync"
	"time"
)

type cache struct {
	mu       sync.Mutex
	held     map[string]entry
	inFlight map[string]*sync.WaitGroup
}

type entry struct {
	value any
	at    time.Time
}

const freshFor = 2 * time.Second

func newCache() *cache {
	return &cache{held: map[string]entry{}, inFlight: map[string]*sync.WaitGroup{}}
}

func (c *cache) get(key string, produce func() any) any {
	for {
		c.mu.Lock()
		if held, ok := c.held[key]; ok && time.Since(held.at) < freshFor {
			c.mu.Unlock()
			return held.value
		}
		if waiting, running := c.inFlight[key]; running {
			c.mu.Unlock()
			waiting.Wait()
			continue
		}

		wg := &sync.WaitGroup{}
		wg.Add(1)
		c.inFlight[key] = wg
		c.mu.Unlock()

		value := produce()

		c.mu.Lock()
		c.held[key] = entry{value: value, at: time.Now()}
		delete(c.inFlight, key)
		c.mu.Unlock()
		wg.Done()

		return value
	}
}

func (c *cache) forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.held = map[string]entry{}
}
