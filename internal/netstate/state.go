package netstate

import "errors"

type Role string

const (
	RoleIngress Role = "ingress"
	RoleEgress  Role = "egress"
)

const (
	QDHeaderLen = 8
	ipv4HdrLen  = 20
	udpHdrLen   = 8
	EncapLen    = ipv4HdrLen + udpHdrLen + QDHeaderLen
)

var (
	ErrNoSuchNode  = errors.New("netstate: no such node")
	ErrNodeOff     = errors.New("netstate: node is disabled")
	ErrUnknownRole = errors.New("netstate: unknown role")
)

type Node struct {
	ID      int    `json:"id"`
	Tag     string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	Role    Role   `json:"role"`
	Enable  bool   `json:"enable"`
	UUID    string `json:"uuid,omitempty"`

	DNSPrimary   string `json:"dnsPrimary"`
	DNSSecondary string `json:"dnsSecondary"`

	Authority string `json:"authority"`
	CertPath  string `json:"certPath"`
	KeyPath   string `json:"keyPath"`
}

type Entrypoint struct {
	ID     int    `json:"id"`
	NodeID int    `json:"nodeId"`
	Port   int    `json:"port"`
	Remark string `json:"remark"`
	Enable bool   `json:"enable"`
}

type Group struct {
	ID            int    `json:"id"`
	Tag           string `json:"name"`
	AllowExit     bool   `json:"allowExit"`
	DeviceLimit   int    `json:"deviceLimit"`
	EntrypointIDs []int  `json:"entrypointIds"`
}

type Client struct {
	ID          int    `json:"id"`
	Tag         string `json:"email"`
	UUID        string `json:"uuid"`
	GroupID     int    `json:"groupId"`
	Enable      bool   `json:"enable"`
	ExpiryAt    int64  `json:"expiryTime"`
	Comment     string `json:"comment"`
	Admin       bool   `json:"admin"`
	DeviceLimit int    `json:"limitIp"`
	AllowExit   int    `json:"allowExit"`
	CreatedAt   int64  `json:"createdAt"`
}

type State struct {
	Revision    int
	Nodes       []Node
	Entrypoints []Entrypoint
	Groups      []Group
	Clients     []Client
	NetworkKey  string
}

const (
	ExitInherit = 0
	ExitAllow   = 1
	ExitDeny    = 2
)

func (c Client) MayExit(g *Group) bool {
	switch c.AllowExit {
	case ExitAllow:
		return true
	case ExitDeny:
		return false
	}
	return g != nil && g.AllowExit
}
