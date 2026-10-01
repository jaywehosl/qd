package clientapi

import (
	"errors"

	"github.com/jaywehosl/qd/internal/qdcrypt"
	"github.com/jaywehosl/qd/internal/qsrv/uplink/relay"
	"github.com/jaywehosl/qd/internal/update"
)

type Device struct {
	ID       string `json:"device"`
	Platform string `json:"platform"`
	Model    string `json:"model"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
}

type Process struct {
	Name        string `json:"name"`
	Path        string `json:"path,omitempty"`
	Icon        string `json:"icon,omitempty"`
	PID         int    `json:"pid"`
	Connections int    `json:"connections"`
}

type Platform interface {
	Running() bool
	Start(servers []string, relays []relay.Link, session uint32) error
	ServerName() string
	Stop() error
	SetKey(key *qdcrypt.Key)
	SetExit(egress bool)
	SetFixedRate(mbit int)
	SyncControlRelays(relays []relay.Link)
	Wire() Asker
	Identify() Device
	Processes() []Process
	RulesChanged()
	HoldAutostart(on bool) error
	Install(tag string, open update.Opener, tick func(done, total int64)) error
}

var ErrStopped = errors.New("stopped before the tunnel came up")
