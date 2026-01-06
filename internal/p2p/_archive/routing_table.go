package p2p

import (
	"sync"
	"time"
)

type RouteTable struct {
	mu     sync.RWMutex
	routes map[string]*Route
}

// NewRouteTable constructs an empty routing table.
func NewRouteTable() *RouteTable {
	return &RouteTable{
		routes: make(map[string]*Route),
	}
}

// internal helper: insert/update a route with distance + TTL refresh.
func (rt *RouteTable) setRoute(peerID, nextHop string, distance int) {
	if peerID == "" || nextHop == "" {
		return
	}
	if distance <= 0 {
		distance = 1
	}

	if rt.routes == nil {
		rt.routes = make(map[string]*Route)
	}
	now := time.Now().Unix()

	if existing, ok := rt.routes[peerID]; ok {
		// prefer shorter paths, always refresh TTL
		if distance < existing.Distance {
			existing.Distance = distance
			existing.NextHop = nextHop
		}
		existing.LastUpdate = now
		return
	}

	rt.routes[peerID] = &Route{
		PeerID:     peerID,
		NextHop:    nextHop,
		Distance:   distance,
		LastUpdate: now,
	}
}

// UpdateDirect records/refreshes a direct neighbor route.
// It accepts a flexible argument list to stay compatible with older callers, e.g.:
//   UpdateDirect(id)
//   UpdateDirect(id, nextHop)
//   UpdateDirect(id, nextHop, distance)
func (rt *RouteTable) UpdateDirect(peerID string, rest ...interface{}) {
	if peerID == "" {
		return
	}

	nextHop := peerID
	distance := 1

	if len(rest) >= 1 {
		if s, ok := rest[0].(string); ok && s != "" {
			nextHop = s
		}
	}
	if len(rest) >= 2 {
		switch v := rest[1].(type) {
		case int:
			if v > 0 {
				distance = v
			}
		case int32:
			if v > 0 {
				distance = int(v)
			}
		case int64:
			if v > 0 {
				distance = int(v)
			}
		case float64:
			if v > 0 {
				distance = int(v)
			}
		}
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.setRoute(peerID, nextHop, distance)
}

// UpdateFromPeerList merges a neighbors advertised routes into the table.
// We keep the signature flexible: the first extra arg is expected to be []Route.
func (rt *RouteTable) UpdateFromPeerList(fromID string, rest ...interface{}) {
	if fromID == "" || len(rest) == 0 {
		return
	}

	routes, ok := rest[0].([]Route)
	if !ok {
		// If the caller shape ever changes, this wont panic, it just no-ops.
		return
	}

	via := fromID

	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Neighbor itself is always distance 1 through itself.
	rt.setRoute(fromID, via, 1)

	for _, r := range routes {
		if r.PeerID == "" {
			continue
		}
		dist := r.Distance
		if dist <= 0 {
			dist = 1
		}
		// Extra hop to get from us -> via -> r.PeerID
		rt.setRoute(r.PeerID, via, dist+1)
	}
}

// NextHopFor returns the next hop for a given destination, if the route is still fresh.
func (rt *RouteTable) NextHopFor(peerID string) (string, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	r, ok := rt.routes[peerID]
	if !ok || !r.Fresh() {
		return "", false
	}
	return r.NextHop, true
}

// Snapshot returns a copy of all fresh routes.
func (rt *RouteTable) Snapshot() []Route {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	out := make([]Route, 0, len(rt.routes))
	for _, r := range rt.routes {
		if r.Fresh() {
			out = append(out, *r)
		}
	}
	return out
}

// StartAgingLoop periodically drops expired routes using Route.Fresh().
func (rt *RouteTable) StartAgingLoop(periodSeconds int) {
	if periodSeconds <= 0 {
		periodSeconds = 30
	}

	go func() {
		ticker := time.NewTicker(time.Duration(periodSeconds) * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			rt.mu.Lock()
			for id, r := range rt.routes {
				if !r.Fresh() {
					delete(rt.routes, id)
				}
			}
			rt.mu.Unlock()
		}
	}()
}

