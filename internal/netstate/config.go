package netstate

// NodeConfig is what one node receives. It is written to a file next to the
// running revision and swapped in by rename, so the encoding has to be stable:
// the same state must produce the same bytes, or the checksum that guards
// `apply` would reject a re-push of identical content.
type NodeConfig struct {
	Revision int       `json:"revision"`
	Self     Self      `json:"self"`
	Ents     []CfgEnt  `json:"entrypoints"`
	Peers    []CfgPeer `json:"peers"`
	Admins   []string  `json:"admins"`
	// Absent for egress, not empty: an egress that receives this key at all is
	// a projection bug, and `omitempty` is what makes the difference visible in
	// the encoded form.
	Clients []CfgClient `json:"clients,omitempty"`
}

type Self struct {
	NodeID int  `json:"nodeId"`
	Role   Role `json:"role"`
	// Bytes one packet grows by on this node's path, already multiplied by the
	// number of encapsulations it performs. The node clamps MSS by this rather
	// than assuming a single hop.
	EncapOverhead int `json:"encapOverhead"`
}

type CfgEnt struct {
	Port int `json:"port"`
}

// CfgClient is the whole of what a node needs to admit a client: who it is,
// until when, and whether it may ask for an exit. No tag, no comment, no
// counters — none of that is the node's business.
type CfgClient struct {
	UUID      string `json:"uuid"`
	ExpiryAt  int64  `json:"expiryAt"`
	AllowExit bool   `json:"allowExit"`
}

// CfgPeer is another node this one may talk to. An ingress is told where to
// reach its egresses; an egress is only told which ingresses to accept, because
// it never dials out.
type CfgPeer struct {
	NodeID  int    `json:"nodeId"`
	Role    Role   `json:"role"`
	Session uint32 `json:"session"`
	Address string `json:"address,omitempty"`
	Port    int    `json:"port,omitempty"`
}
