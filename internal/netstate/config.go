package netstate

type NodeConfig struct {
	Revision int         `json:"revision"`
	Self     Self        `json:"self"`
	Ents     []CfgEnt    `json:"entrypoints"`
	Peers    []CfgPeer   `json:"peers"`
	Admins   []string    `json:"admins"`
	Clients  []CfgClient `json:"clients,omitempty"`
}

type Self struct {
	NodeID        int  `json:"nodeId"`
	Role          Role `json:"role"`
	EncapOverhead int  `json:"encapOverhead"`
}

type CfgEnt struct {
	Port int `json:"port"`
}

type CfgClient struct {
	UUID      string `json:"uuid"`
	ExpiryAt  int64  `json:"expiryAt"`
	AllowExit bool   `json:"allowExit"`
}

type CfgPeer struct {
	NodeID  int    `json:"nodeId"`
	Role    Role   `json:"role"`
	Session uint32 `json:"session"`
	Address string `json:"address,omitempty"`
	Port    int    `json:"port,omitempty"`
}
