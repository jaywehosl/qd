package clientapi

import (
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
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
	Start(servers []string, session uint32) error
	ServerName() string
	Stop() error
	SetKey(key *qdcrypt.Key)
	SetExit(egress bool)
	SetFixedRate(mbit int)
	Wire() Asker
	Identify() Device
	Processes() []Process
	RulesChanged()
	HoldAutostart(on bool) error
	AutostartHeld() bool
}
