package mesh

import (
	"net"
	"strings"
	"time"
)

func normalizeHost(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return h
	}
	// Treat common loopback spellings as equivalent.
	switch strings.ToLower(h) {
	case "localhost":
		return "127.0.0.1"
	}
	// Treat bind-all as wildcard-ish for matching.
	if h == "0.0.0.0" || h == "::" {
		return ""
	}
	if h == "::1" {
		return "127.0.0.1"
	}
	return h
}

func splitHostPortLoose(addr string) (host string, port string, ok bool) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", "", false
	}
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", false
	}
	return normalizeHost(h), strings.TrimSpace(p), true
}

func equivalentPeerAddr(a, b string) bool {
	ha, pa, oka := splitHostPortLoose(a)
	hb, pb, okb := splitHostPortLoose(b)
	if !oka || !okb {
		// Fallback exact match if parsing fails
		return strings.TrimSpace(a) == strings.TrimSpace(b)
	}
	if pa == "" || pb == "" || pa != pb {
		return false
	}

	// If either host is wildcard/empty, match on port.
	if ha == "" || hb == "" {
		return true
	}

	// Exact host match after normalization.
	return ha == hb
}

// TouchPeer = ACTIVITY (real traffic only)
func (m *meshDaemon) TouchPeer(addr string) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return
	}

	for _, bp := range m.bootstrapPeers {
		if equivalentPeerAddr(bp, addr) {
			m.lock.Lock()
			defer m.lock.Unlock()

			// Store by the canonical bootstrap key to keep /peers stable.
			key := strings.TrimSpace(bp)
			if key == "" {
				key = addr
			}

			p, ok := m.peers[key]
			if !ok {
				p = &Peer{Addr: key}
				m.peers[key] = p
			}

			p.Connected = true
			p.LastSeen = time.Now()
			return
		}
	}
}

// TouchReachable = DIAL success only (NO activity)
func (m *meshDaemon) TouchReachable(addr string, ok bool) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return
	}

	for _, bp := range m.bootstrapPeers {
		if equivalentPeerAddr(bp, addr) {
			m.lock.Lock()
			defer m.lock.Unlock()

			key := strings.TrimSpace(bp)
			if key == "" {
				key = addr
			}

			p, exists := m.peers[key]
			if !exists {
				p = &Peer{Addr: key}
				m.peers[key] = p
			}

			p.Reachable = ok
			return
		}
	}
}
