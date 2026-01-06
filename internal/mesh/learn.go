package mesh

// LearnPeer is kept for compatibility; it now funnels into ACTIVITY-based TouchPeer.
// IMPORTANT: LearnPeer should only be called on real inbound traffic (messages.go uses Via).
func (m *meshDaemon) LearnPeer(addr string) {
	m.TouchPeer(addr)
}
