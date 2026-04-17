package mesh

import (
	"net"
	"strings"
	"time"
)

const (
	peerFailureSuppressAfter = 3
	peerFailureSuppressStep  = 15 * time.Second
	peerFailureSuppressMax   = 60 * time.Second
)

func normalizeHost(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return h
	}
	switch strings.ToLower(h) {
	case "localhost":
		return "127.0.0.1"
	}
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
		return strings.TrimSpace(a) == strings.TrimSpace(b)
	}
	if pa == "" || pb == "" || pa != pb {
		return false
	}
	if ha == "" || hb == "" {
		return true
	}
	return ha == hb
}

// TouchPeer = real traffic observed.
func (m *meshDaemon) TouchPeer(addr string) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	p, ok := m.peers[addr]
	if !ok {
		p = &Peer{Addr: addr}
		m.peers[addr] = p
	}
	p.Connected = true
	p.LastSeen = time.Now()
	p.FailureCount = 0
	p.SuppressedUntil = time.Time{}
}

// TouchReachable = dial success/failure observed.
func (m *meshDaemon) TouchReachable(addr string, ok bool) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	p, exists := m.peers[addr]
	if !exists {
		p = &Peer{Addr: addr}
		m.peers[addr] = p
	}
	p.Reachable = ok
	if ok {
		p.FailureCount = 0
		p.SuppressedUntil = time.Time{}
		return
	}
	p.FailureCount++
	if p.FailureCount >= peerFailureSuppressAfter {
		steps := p.FailureCount - peerFailureSuppressAfter + 1
		suppressFor := time.Duration(steps) * peerFailureSuppressStep
		if suppressFor > peerFailureSuppressMax {
			suppressFor = peerFailureSuppressMax
		}
		p.SuppressedUntil = time.Now().Add(suppressFor)
	}
}

func (m *meshDaemon) shouldDialPeer(addr string, now time.Time) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}
	m.lock.RLock()
	defer m.lock.RUnlock()
	p, ok := m.peers[addr]
	if !ok {
		return true
	}
	return p.SuppressedUntil.IsZero() || !now.Before(p.SuppressedUntil)
}
