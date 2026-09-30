package clientapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jaywehosl/qd/internal/update"
)

var ErrUpToDate = errors.New("this is the newest version the network offers")

const postponeKey = "updatePostpone"

type offerAt struct {
	Offer
	endpoint string
}

func (a *API) keepOffer(o *Offer, endpoint string) {
	a.upMu.Lock()
	defer a.upMu.Unlock()
	if a.fake != "" {
		return
	}
	if o == nil || o.Version == "" || o.State == update.Current || !update.Older(update.Version, o.Version) {
		a.offer = nil
		return
	}
	a.offer = &offerAt{Offer: *o, endpoint: endpoint}
}

func (a *API) updateState(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if err := a.CheckAndUpdate(); err != nil && !errors.Is(err, ErrUpToDate) {
			fail(w, err)
			return
		}
	}
	ok(w, a.UpdateInfo())
}

func (a *API) postponeUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Minutes int `json:"minutes"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := a.Postpone(body.Minutes); err != nil {
		fail(w, err)
		return
	}
	ok(w, a.UpdateInfo())
}

func (a *API) UpdateInfo() map[string]any {
	postponed := a.postponed()
	a.upMu.Lock()
	defer a.upMu.Unlock()
	out := map[string]any{
		"version": update.Version,
		"build":   update.Kind,
		"status":  a.upStatus,
		"error":   a.upErr,
		"done":    a.upDone,
		"total":   a.upTotal,
		"failed":  a.upFailed,
	}
	if a.offer != nil {
		out["offer"] = a.offer.Offer
		out["release"] = update.ReleaseURL(a.offer.Version)
		if a.offer.State != update.Required && postponed.version == a.offer.Version {
			out["postponedUntil"] = postponed.until
		}
	}
	if a.updatedFrom != "" && time.Since(a.updatedAt) < updatedShown {
		out["updatedFrom"] = a.updatedFrom
		a.updatedFrom = ""
	}
	return out
}

func (a *API) CheckAndUpdate() error {
	a.sweep()
	return a.StartUpdate()
}

func (a *API) StartUpdate() error {
	a.upMu.Lock()
	defer a.upMu.Unlock()
	switch {
	case a.upStatus != "":
		return errors.New("an update is already under way")
	case a.offer == nil:
		return ErrUpToDate
	}
	held := *a.offer
	a.upStatus, a.upErr, a.upFailed, a.upDone, a.upTotal = "fetching", "", "", 0, 0
	go a.runUpdate(held)
	return nil
}

func (a *API) runUpdate(o offerAt) {
	open := func(name string) (io.ReadCloser, error) {
		return a.platform.Wire().Open(o.endpoint, update.Path+o.Version+"/"+name)
	}
	tick := func(done, total int64) {
		a.upMu.Lock()
		a.upDone, a.upTotal = done, total
		a.upMu.Unlock()
	}
	a.upMu.Lock()
	fake := a.fake != ""
	corrupt := a.fakeCorrupt
	a.upMu.Unlock()
	var err error
	if fake {
		err = fakeInstall(tick, corrupt)
	} else {
		err = a.platform.Install(o.Version, open, tick)
	}

	a.upMu.Lock()
	defer a.upMu.Unlock()
	if err != nil {
		a.upStatus, a.upErr = "", err.Error()
		if errors.Is(err, update.ErrCorrupt) {
			a.upFailed = "corrupt"
		}
		a.db.Notify("error", "The update to "+o.Version+" failed: "+err.Error(), time.Now().UnixMilli())
		return
	}
	a.upStatus = "installing"
	wait := installWait
	if fake {
		wait = 3 * time.Second
	}
	time.AfterFunc(wait, func() {
		a.upMu.Lock()
		if a.upStatus == "installing" {
			a.upStatus = ""
			if fake {
				a.offer, a.fake = nil, ""
				a.updatedFrom, a.updatedAt = update.Version, time.Now()
			}
		}
		a.upMu.Unlock()
	})
}

const (
	installWait  = 2 * time.Minute
	updatedShown = 2 * time.Minute
)

func (a *API) Postpone(minutes int) error {
	a.upMu.Lock()
	held := a.offer
	unit := time.Minute
	if a.fake != "" {
		unit = time.Second
	}
	a.upMu.Unlock()
	switch {
	case held == nil:
		return ErrUpToDate
	case held.State == update.Required:
		return errors.New("this update cannot be put off")
	}
	until := int64(-1)
	if minutes > 0 {
		until = time.Now().Add(time.Duration(minutes) * unit).UnixMilli()
	}
	return a.db.SetValue(postponeKey, held.Version+"|"+strconv.FormatInt(until, 10))
}

type putOff struct {
	version string
	until   int64
}

func (a *API) postponed() putOff {
	raw, _ := a.db.Value(postponeKey)
	version, until, found := strings.Cut(raw, "|")
	if !found {
		return putOff{}
	}
	ms, err := strconv.ParseInt(until, 10, 64)
	if err != nil || (ms >= 0 && ms < time.Now().UnixMilli()) {
		return putOff{}
	}
	return putOff{version: version, until: ms}
}

const fakeVersion = "v9.9.9-alpha"

func (a *API) Fake(state string) {
	corrupt := state == "corrupt"
	if corrupt {
		state = string(update.Behind)
	}
	verdict := update.Verdict(state)
	if update.Version != "dev" || (verdict != update.Behind && verdict != update.Required) {
		return
	}
	a.db.SetValue(postponeKey, "")
	a.upMu.Lock()
	a.fake = verdict
	a.fakeCorrupt = corrupt
	a.offer = &offerAt{Offer: Offer{Version: fakeVersion, State: verdict}}
	a.upMu.Unlock()
}

func fakeInstall(tick func(done, total int64), corrupt bool) error {
	const total = 24 << 20
	for done := int64(0); done < total; done += total / 80 {
		tick(done, total)
		time.Sleep(100 * time.Millisecond)
	}
	tick(total, total)
	if corrupt {
		return fmt.Errorf("%w: the synthetic package fails its checksum", update.ErrCorrupt)
	}
	return nil
}

const lastVersionKey = "lastVersion"

func (a *API) noteVersion() {
	prev, _ := a.db.Value(lastVersionKey)
	if prev == update.Version {
		return
	}
	a.db.SetValue(lastVersionKey, update.Version)
	if prev != "" {
		a.updatedFrom, a.updatedAt = prev, time.Now()
	}
}
