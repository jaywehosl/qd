package panel

import (
	"encoding/json"
	"fmt"
	"time"
)

func (a *API) Converge(stop <-chan struct{}, open func() bool) {
	tick := time.NewTicker(convergeEvery)
	defer tick.Stop()

	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			if open != nil && !open() {
				continue
			}
			a.evenOut()
		}
	}
}

func (a *API) evenOut() {
	health := a.fleet.Health()
	if len(health) < 2 {
		return
	}

	top, from := -1, 0
	for _, h := range health {
		if h.Online && h.Revision > top {
			top, from = h.Revision, h.ID
		}
	}
	if top < 0 {
		return
	}

	behind := []NodeHealth{}
	for _, h := range health {
		if h.Online && h.Revision < top {
			behind = append(behind, h)
		}
	}
	if len(behind) == 0 {
		return
	}

	blob, err := a.pullDB(from)
	if err != nil {
		fmt.Printf("fleet    could not read the database to spread: %v\n", err)
		return
	}

	for _, n := range behind {
		fmt.Printf("fleet    %s is at revision %d, the network is at %d\n", n.Tag, n.Revision, top)
		if err := a.pushDB(n.ID, blob); err != nil {
			fmt.Printf("fleet    %s did not take the database: %v\n", n.Tag, err)
			continue
		}
		if now, err := a.revisionOf(n.ID); err != nil {
			fmt.Printf("fleet    %s took the database but did not answer after: %v\n", n.Tag, err)
		} else if now < top {
			fmt.Printf("fleet    %s took the database and stayed at revision %d\n", n.Tag, now)
		} else {
			fmt.Printf("fleet    %s caught up to revision %d\n", n.Tag, now)
		}
	}
	a.cache.forget()
}

func (a *API) revisionOf(id int) (int, error) {
	body, err := a.fleet.Ask(id, "hello", nil)
	if err != nil {
		return 0, err
	}
	var said struct {
		Revision int `json:"revision"`
	}
	if err := json.Unmarshal(body, &said); err != nil {
		return 0, err
	}
	return said.Revision, nil
}

const convergeEvery = 30 * time.Second
