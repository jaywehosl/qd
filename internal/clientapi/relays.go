package clientapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/vlesslink"
)

type RelayState struct {
	Reach   int64  `json:"reach"`
	Shake   int64  `json:"shake"`
	Checked int64  `json:"checked"`
	Fault   string `json:"fault,omitempty"`
	Broken  bool   `json:"broken,omitempty"`
}

type relayRow struct {
	ID        int        `json:"id"`
	Enabled   bool       `json:"enabled"`
	Name      string     `json:"name"`
	Server    string     `json:"server"`
	Transport string     `json:"transport"`
	Security  string     `json:"security"`
	State     RelayState `json:"state"`
}

type relayDriver interface {
	SetRelays(list []clientstate.Relay, mode string)
	RelayStates() map[int]RelayState
	ProbeRelays()
}

func (a *API) relayDriver() (relayDriver, bool) {
	held, ok := a.platform.(relayDriver)
	return held, ok
}

func (a *API) applyRelays() {
	held, ok := a.relayDriver()
	if !ok {
		return
	}
	list, err := a.db.Relays()
	if err != nil {
		return
	}
	settings, _ := a.db.Settings()
	held.SetRelays(list, settings.RelayMode)
}

func (a *API) relayList() (map[string]any, error) {
	list, err := a.db.Relays()
	if err != nil {
		return nil, err
	}
	settings, _ := a.db.Settings()
	held, supported := a.relayDriver()
	states := map[int]RelayState{}
	if supported {
		states = held.RelayStates()
	}
	rows := make([]relayRow, 0, len(list))
	for _, r := range list {
		row := relayRow{ID: r.ID, Enabled: r.Enabled, State: states[r.ID]}
		if l, err := vlesslink.Parse(r.Link); err == nil {
			row.Name, row.Server, row.Transport, row.Security = l.Name, l.Server(), l.Transport, l.Security
		} else {
			row.State.Fault, row.State.Broken = err.Error(), true
		}
		rows = append(rows, row)
	}
	return map[string]any{"mode": settings.RelayMode, "supported": supported, "relays": rows}, nil
}

func (a *API) answerRelays(w http.ResponseWriter) {
	list, err := a.relayList()
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, list)
}

func (a *API) relays(w http.ResponseWriter, r *http.Request) { a.answerRelays(w) }

var errPostOnly = errors.New("this call takes POST")

func (a *API) addRelay(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Link string `json:"link"`
	}
	if r.Method != http.MethodPost {
		fail(w, errPostOnly)
		return
	}
	if !decode(w, r, &body) {
		return
	}
	link := strings.TrimSpace(body.Link)
	if _, err := vlesslink.Parse(link); err != nil {
		fail(w, err)
		return
	}
	if _, err := a.db.AddRelay(link); err != nil {
		fail(w, err)
		return
	}
	a.applyRelays()
	if held, ok := a.relayDriver(); ok {
		held.ProbeRelays()
	}
	a.answerRelays(w)
}

func (a *API) enableRelay(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      int  `json:"id"`
		Enabled bool `json:"enabled"`
	}
	if r.Method != http.MethodPost {
		fail(w, errPostOnly)
		return
	}
	if !decode(w, r, &body) {
		return
	}
	if err := a.db.EnableRelay(body.ID, body.Enabled); err != nil {
		fail(w, err)
		return
	}
	a.applyRelays()
	if held, ok := a.relayDriver(); ok && body.Enabled {
		held.ProbeRelays()
	}
	a.answerRelays(w)
}

func (a *API) removeRelay(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID int `json:"id"`
	}
	if r.Method != http.MethodPost {
		fail(w, errPostOnly)
		return
	}
	if !decode(w, r, &body) {
		return
	}
	if err := a.db.DeleteRelay(body.ID); err != nil {
		fail(w, err)
		return
	}
	a.applyRelays()
	a.answerRelays(w)
}

func (a *API) probeRelays(w http.ResponseWriter, r *http.Request) {
	if held, ok := a.relayDriver(); ok {
		held.ProbeRelays()
	}
	a.answerRelays(w)
}
