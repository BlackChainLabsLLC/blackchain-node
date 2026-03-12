package mesh

import (
	"encoding/json"
	"strings"
	"time"
)

func (m *meshDaemon) gossipSignedState() {
	m.chain.mu.RLock()
	snap := m.chain.SignedStateHash()
	m.chain.mu.RUnlock()

	// Defensive conversion (avoid panics)
	var h int64
	switch v := snap["height"].(type) {
	case int64:
		h = v
	case int:
		h = int64(v)
	case float64:
		h = int64(v)
	default:
		h = 0
	}

	sh, _ := snap["state_hash"].(string)
	sig, _ := snap["sig"].(string)

	ann := SignedStateAnnouncement{
		NodeID:    m.nodeID,
		Height:    h,
		StateHash: sh,
		Sig:       sig,
	}

	raw, err := json.Marshal(ann)
	if err != nil {
		return
	}

	// Send as normal "msg" so messages.go accepts it; consumers key off Topic.
	id := "st-" + strings.ReplaceAll(strings.TrimSpace(m.id), ":", "_") + "-" +
		time.Now().UTC().Format("20060102T150405.000000000Z")
	m.gossipOrigin(string(raw), "signed_state", id, 3)
}

// startSignedStateLoop periodically gossips local signed state hash
func (m *meshDaemon) startSignedStateLoop() {
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			m.gossipSignedState()
			<-t.C
		}
	}()
}
