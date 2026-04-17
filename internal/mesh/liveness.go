package mesh

import (
	"log"
	"time"
)

const (
	LivenessWindow = 10 * time.Second
	LivenessTick   = 3 * time.Second
)

// startLivenessLoop enforces ACTIVITY truth only.
func (m *meshDaemon) startLivenessLoop() {
	go func() {
		ticker := time.NewTicker(LivenessTick)
		defer ticker.Stop()

		for range ticker.C {
			select {
			case <-m.runCtx.Done():
				log.Printf("[liveness] stopping node=%s", m.nodeID)
				return
			default:
			}
			now := time.Now()

			m.lock.Lock()
			for _, p := range m.peers {
				if p.Connected && now.Sub(p.LastSeen) > LivenessWindow {
					p.Connected = false
				}
			}
			m.lock.Unlock()
		}
	}()
}
