package mesh

import "time"

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
