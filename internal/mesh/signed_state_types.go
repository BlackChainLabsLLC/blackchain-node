package mesh

// SignedStateAnnouncement is gossiped periodically so peers can compare tip/height.
type SignedStateAnnouncement struct {
	NodeID    string `json:"node_id"`
	Height    int64  `json:"height"`
	StateHash string `json:"state_hash"`
	Sig       string `json:"sig"`
}
