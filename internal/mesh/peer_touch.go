package mesh

import "time"

// TouchPeer = ACTIVITY (real traffic only)
func (m *meshDaemon) TouchPeer(addr string) {
	if addr == "" {
		return
	}

	for _, bp := range m.bootstrapPeers {
		if bp == addr {
			m.lock.Lock()
			defer m.lock.Unlock()

			p, ok := m.peers[addr]
			if !ok {
				p = &Peer{Addr: addr}
				m.peers[addr] = p
			}

			p.Connected = true
			p.LastSeen = time.Now()
			return
		}
	}
}

// TouchReachable = DIAL success only (NO activity)
func (m *meshDaemon) TouchReachable(addr string, ok bool) {
	if addr == "" {
		return
	}

	for _, bp := range m.bootstrapPeers {
		if bp == addr {
			m.lock.Lock()
			defer m.lock.Unlock()

			p, exists := m.peers[addr]
			if !exists {
				p = &Peer{Addr: addr}
				m.peers[addr] = p
			}

			p.Reachable = ok
			return
		}
	}
}
