package mesh

import (
	"time"
)

// Reputation tracks trust metrics for a node (remote peers and this node itself).
type Reputation struct {
	NodeID         string    `json:"node_id"`
	PubKey         string    `json:"pubkey"`
	VerifiedCount  int64     `json:"verified_count"`
	InvalidCount   int64     `json:"invalid_count"`
	ForwardedCount int64     `json:"forwarded_count"`
	Score          int64     `json:"score"`
	LastSeen       time.Time `json:"last_seen"`
}

// UCRecord tracks Utility Credits for a node.
type UCRecord struct {
	NodeID string `json:"node_id"`
	PubKey string `json:"pubkey"`
	UC     int64  `json:"uc"`
}

// noteReputationFromWire updates reputation/UC for the sender of a wireMessage.
func (m *meshDaemon) noteReputationFromWire(wm *wireMessage, verified bool) {
	if wm == nil || wm.PubKey == "" {
		return
	}

	nodeID := wm.PubKey // treat sender pubkey hex as node_id
	pub := wm.PubKey

	m.repMu.Lock()
	defer m.repMu.Unlock()

	rep, ok := m.reputation[nodeID]
	if !ok {
		rep = &Reputation{
			NodeID: nodeID,
			PubKey: pub,
		}
		m.reputation[nodeID] = rep
	}

	rep.LastSeen = time.Now()
	if verified {
		rep.VerifiedCount++
	} else {
		rep.InvalidCount++
	}

	// Simple score: verified - invalid + forwarded
	rep.Score = rep.VerifiedCount + rep.ForwardedCount - rep.InvalidCount

	// Award 1 UC per verified message.
	if verified {
		uc, ok := m.uc[nodeID]
		if !ok {
			uc = &UCRecord{
				NodeID: nodeID,
				PubKey: pub,
			}
			m.uc[nodeID] = uc
		}
		uc.UC++
	}
}

// noteForwardCredit records that THIS node successfully forwarded/originated
// a signed message and mints 1 UC for the local node.
func (m *meshDaemon) noteForwardCredit() {
	if m == nil || m.nodeID == "" {
		return
	}

	nodeID := m.nodeID
	pub := nodeID // identity.go already encodes pubkey as hex for nodeID

	m.repMu.Lock()
	defer m.repMu.Unlock()

	rep, ok := m.reputation[nodeID]
	if !ok {
		rep = &Reputation{
			NodeID: nodeID,
			PubKey: pub,
		}
		m.reputation[nodeID] = rep
	}

	rep.ForwardedCount++
	rep.LastSeen = time.Now()
	rep.Score = rep.VerifiedCount + rep.ForwardedCount - rep.InvalidCount

	uc, ok := m.uc[nodeID]
	if !ok {
		uc = &UCRecord{
			NodeID: nodeID,
			PubKey: pub,
		}
		m.uc[nodeID] = uc
	}
	uc.UC++
}

// snapshotReputation returns a copy of the reputation table for HTTP.
func (m *meshDaemon) snapshotReputation() []*Reputation {
	m.repMu.Lock()
	defer m.repMu.Unlock()

	out := make([]*Reputation, 0, len(m.reputation))
	for _, rep := range m.reputation {
		cp := *rep
		out = append(out, &cp)
	}
	return out
}

// snapshotUC returns a copy of UC balances for HTTP.
func (m *meshDaemon) snapshotUC() []*UCRecord {
	m.repMu.Lock()
	defer m.repMu.Unlock()

	out := make([]*UCRecord, 0, len(m.uc))
	for _, rec := range m.uc {
		cp := *rec
		out = append(out, &cp)
	}
	return out
}
