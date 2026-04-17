package mesh

import (
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	peerFailureSuppressAfter = 3
	peerFailureSuppressStep  = 15 * time.Second
	peerFailureSuppressMax   = 60 * time.Second
	syncFailureSuppressStep  = 2 * time.Second
	syncFailureSuppressMax   = 30 * time.Second
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

func sanitizeLearnedPeerAddr(addr, selfAddr string, existing map[string]*Peer) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", fmt.Errorf("learned peer addr is empty")
	}
	normalized, err := validateTCPAddress("learned peer", addr)
	if err != nil {
		return "", err
	}
	host, port, ok := splitHostPortLoose(normalized)
	if !ok || port == "" {
		return "", fmt.Errorf("learned peer addr is invalid: %s", addr)
	}
	if host == "" {
		return "", fmt.Errorf("learned peer must not use wildcard listen: %s", addr)
	}
	normalized = net.JoinHostPort(host, port)
	if selfAddr != "" && equivalentPeerAddr(normalized, selfAddr) {
		return "", fmt.Errorf("learned peer must not contain self address: %s", addr)
	}
	for existingAddr := range existing {
		if equivalentPeerAddr(normalized, existingAddr) {
			return existingAddr, nil
		}
	}
	return normalized, nil
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
		p.SyncFailures = 0
		p.LastSyncErr = ""
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

func (m *meshDaemon) noteSyncFailure(addr string, err error) time.Duration {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return 0
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	p, exists := m.peers[addr]
	if !exists {
		p = &Peer{Addr: addr}
		m.peers[addr] = p
	}
	p.SyncFailures++
	if err != nil {
		p.LastSyncErr = err.Error()
	}
	suppressFor := time.Duration(p.SyncFailures) * syncFailureSuppressStep
	if suppressFor > syncFailureSuppressMax {
		suppressFor = syncFailureSuppressMax
	}
	p.SuppressedUntil = time.Now().Add(suppressFor)
	return suppressFor
}

func (m *meshDaemon) noteSyncSuccess(addr string, height int64, tip string) {
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
	p.SyncFailures = 0
	p.LastSyncErr = ""
	p.LastHeight = height
	p.LastTip = strings.TrimSpace(tip)
	p.SuppressedUntil = time.Time{}
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
