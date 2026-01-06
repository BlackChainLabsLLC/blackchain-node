package p2p

import "time"

type Route struct {
    PeerID     string
    NextHop    string
    Distance   int
    LastUpdate int64 // unix stamp
}

const ROUTE_TTL = 45 * time.Second

// 🔥 Routes expire automatically unless refreshed
func (r Route) Fresh() bool {
    return time.Since(time.Unix(r.LastUpdate, 0)) < ROUTE_TTL
}

