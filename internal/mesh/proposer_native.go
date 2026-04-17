package mesh

import (
	"log"
	"time"
)

func (m *meshDaemon) startProposerLoop() {
	if m.nodeID != "node1" {
		log.Printf("[proposer] disabled on %s (single-proposer mode)", m.nodeID)
		return
	}

	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			select {
			case <-m.runCtx.Done():
				log.Printf("[proposer] stopping node=%s", m.nodeID)
				return
			default:
			}
			m.chain.mu.Lock()

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
