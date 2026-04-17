package mesh

import (
	"log"
	"time"
)

func (m *meshDaemon) proposerReadyLocked() error {
	return m.chain.requireValidatorActionReadyLocked(m.nodeID)
}

func (m *meshDaemon) startProposerLoop() {
	if m.nodeID != "node1" {
		log.Printf("[proposer] disabled on %s (single-proposer mode)", m.nodeID)
		return
	}

	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			m.chain.mu.Lock()
			if err := m.proposerReadyLocked(); err != nil {
				log.Printf("[proposer] validator action not ready: %v", err)
				m.chain.mu.Unlock()
				continue
			}

			before := m.chain.height
			err := m.chain.proposeBlock()
			after := m.chain.height

			if err != nil {
				log.Printf("[proposer] propose error: %v", err)
				m.chain.mu.Unlock()
				continue
			}

			var out Block
			if after > before {
				out = m.chain.blocks[after]
				log.Printf("[proposer] block height=%d", after)
			} else {
				log.Printf("[proposer] no-op height=%d", after)
			}

			m.chain.mu.Unlock()

			if after > before {
				go m.gossipBlock(out, 3)
			}
		}
	}()
}
