package netstate

type NodeConfig struct {
	Clients []CfgClient
	Peers   []CfgPeer
}

type CfgClient struct {
	UUID      string
	ExpiryAt  int64
	AllowExit bool
	RouteDNS  bool
}

type CfgPeer struct {
	NodeID  int
	Role    Role
	Session uint32
}
