package netstate

type NodeConfig struct {
	Clients []CfgClient
	Peers   []CfgPeer
}

type CfgClient struct {
	ID          int
	UUID        string
	ExpiryAt    int64
	DeviceLimit int
	AllowExit   bool
	AllowDev    bool
	AllowCore   bool
}

type CfgPeer struct {
	NodeID  int
	Role    Role
	Session uint32
}
