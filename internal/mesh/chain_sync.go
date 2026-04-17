package mesh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

/*
BOOTSTRAP SYNC
- Poll reachable peers
- Ask heights
- Pick highest
- Pull blocks
- Apply
*/

func (m *meshDaemon) bootstrapSync(ctx context.Context) {
	const (
		minRetryDelay        = 300 * time.Millisecond
		maxRetryDelay        = 5 * time.Second
		maxSyncBackoff       = 30 * time.Second
		rangeChunk     int64 = 50
	)
	retryDelay := minRetryDelay

	// Gate chain sync until at least one peer is reachable
	for {
		peers := m.reachablePeersForSync()
		if len(peers) > 0 {
			break
		}
		time.Sleep(minRetryDelay)
	}

	log.Println("[sync] reachable peers detected, starting sync")
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[sync] panic: %v", r)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		peers := m.reachablePeersForSync()

		if len(peers) == 0 {
			time.Sleep(retryDelay)
			retryDelay = nextRetryDelay(retryDelay, maxRetryDelay)
			continue
		}

		log.Printf("[sync] probing peers: %v", peers)

		type cand struct {
			Addr   string
			Height int64
			Tip    string
		}

		var cands []cand

		for _, addr := range peers {
			h, tip, err := m.requestPeerHeight(addr)
			if err != nil {
				log.Printf("[sync] height err %s: %v", addr, err)
				m.noteSyncFailure(addr, "probe-height", maxSyncBackoff, err)
				continue
			}
			if !m.notePeerHeightSample(addr, h, tip, maxSyncBackoff) {
				continue
			}
			cands = append(cands, cand{addr, h, tip})
		}

		if len(cands) == 0 {
			time.Sleep(retryDelay)
			retryDelay = nextRetryDelay(retryDelay, maxRetryDelay)
			continue
		}

		sort.Slice(cands, func(i, j int) bool {
			return cands[i].Height > cands[j].Height
		})

		best := cands[0]

		m.chain.mu.RLock()
		localH := m.chain.height
		m.chain.mu.RUnlock()

		if best.Height <= localH {
			m.noteSyncSuccess(best.Addr)
			log.Printf("[sync] up-to-date local=%d best=%d", localH, best.Height)
			time.Sleep(1 * time.Second)
			retryDelay = minRetryDelay
			continue
		}

		log.Printf("[sync] behind local=%d best=%d @ %s", localH, best.Height, best.Addr)

		syncErr := false

		for cur := localH + 1; cur <= best.Height; cur += rangeChunk {

			end := cur + rangeChunk - 1
			if end > best.Height {
				end = best.Height
			}

			blocks, err := m.requestPeerRange(best.Addr, cur, end)
			if err != nil {
				log.Printf("[sync] range err peer=%s from=%d to=%d: %v", best.Addr, cur, end, err)
				m.noteSyncFailure(best.Addr, "fetch-range", maxSyncBackoff, err)
				syncErr = true
				break
			}
			if len(blocks) == 0 {
				m.noteSyncFailure(best.Addr, "empty-range", maxSyncBackoff, fmt.Errorf("peer returned no blocks"))
				syncErr = true
				break
			}

			m.chain.mu.Lock()
			for _, b := range blocks {
				if _, err := m.chain.applyBlockOrBufferLocked(b); err != nil {
					m.chain.mu.Unlock()
					m.noteSyncFailure(best.Addr, "apply-block", maxSyncBackoff, err)
					syncErr = true
					break
				}
			}
			if syncErr {
				break
			}
			_ = m.chain.drainPendingLocked()
			m.chain.mu.Unlock()
		}
		if syncErr {
			time.Sleep(retryDelay)
			retryDelay = nextRetryDelay(retryDelay, maxRetryDelay)
			continue
		}

		m.chain.mu.RLock()
		h := m.chain.height
		t := m.chain.tip
		m.chain.mu.RUnlock()

		m.noteSyncSuccess(best.Addr)
		log.Printf("[sync] done height=%d tip=%s", h, t)
		// NOTE: do not return; keep syncing for future blocks
		time.Sleep(750 * time.Millisecond)
		retryDelay = minRetryDelay
		continue
	}
}

func nextRetryDelay(cur, max time.Duration) time.Duration {
	if cur <= 0 {
		return 300 * time.Millisecond
	}
	next := time.Duration(math.Min(float64(max), float64(cur*2)))
	if next < 300*time.Millisecond {
		return 300 * time.Millisecond
	}
	return next
}

func (m *meshDaemon) notePeerHeightSample(addr string, h int64, tip string, maxBackoff time.Duration) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	p, ok := m.peers[addr]
	if !ok {
		return false
	}
	tip = strings.TrimSpace(tip)
	if h <= 0 || tip == "" {
		p.ObservedState = "sync_probe_invalid"
		p.SyncFailures++
		p.LastSyncErr = "invalid advertised height/tip"
		p.SuppressedUntil = time.Now().Add(backoffForFailures(p.SyncFailures, maxBackoff))
		return false
	}

	// Inconsistent reporting: tip changed without advancing height.
	if p.LastHeight > 0 && h == p.LastHeight && p.LastTip != "" && p.LastTip != tip {
		p.ObservedState = "sync_probe_inconsistent"
		p.SyncFailures++
		p.LastSyncErr = "tip changed at same height"
		p.SuppressedUntil = time.Now().Add(backoffForFailures(p.SyncFailures, maxBackoff))
		return false
	}

	// Stale backwards reporting beyond minor jitter.
	if p.LastHeight > 0 && h+1 < p.LastHeight {
		p.ObservedState = "sync_probe_stale"
		p.SyncFailures++
		p.LastSyncErr = fmt.Sprintf("height regressed from %d to %d", p.LastHeight, h)
		p.SuppressedUntil = time.Now().Add(backoffForFailures(p.SyncFailures, maxBackoff))
		return false
	}

	p.LastHeight = h
	p.LastTip = tip
	return true
}

func (m *meshDaemon) noteSyncFailure(addr, state string, maxBackoff time.Duration, err error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	p, ok := m.peers[addr]
	if !ok {
		return
	}
	p.SyncFailures++
	p.ObservedState = state
	if err != nil {
		p.LastSyncErr = err.Error()
	}
	p.SuppressedUntil = time.Now().Add(backoffForFailures(p.SyncFailures, maxBackoff))
}

func (m *meshDaemon) noteSyncSuccess(addr string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	p, ok := m.peers[addr]
	if !ok {
		return
	}
	p.SyncFailures = 0
	p.LastSyncErr = ""
	p.SuppressedUntil = time.Time{}
	if p.ObservedState == "" || strings.HasPrefix(p.ObservedState, "sync_") {
		p.ObservedState = "sync_ok"
	}
}

func backoffForFailures(failures int, max time.Duration) time.Duration {
	if failures <= 0 {
		return 0
	}
	sec := math.Pow(2, float64(failures-1))
	wait := time.Duration(sec) * time.Second
	if wait > max {
		return max
	}
	return wait
}

func (m *meshDaemon) requestPeerHeight(addr string) (int64, string, error) {
	// Derive peer HTTP port from peer mesh listen port.
	// Canonical local dev pairing:
	//   mesh 7072 -> http 6060
	//   mesh 7073 -> http 6061
	// i.e. httpPort = meshPort - 1012
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return 0, "", fmt.Errorf("bad peer addr (no port): %q", addr)
	}
	host := addr[:i]
	pstr := addr[i+1:]
	meshPort, err := strconv.Atoi(pstr)
	if err != nil {
		return 0, "", fmt.Errorf("bad peer port %q: %w", pstr, err)
	}
	httpPort := meshPort - 1012
	if httpPort <= 0 {
		return 0, "", fmt.Errorf("derived bad http port from %d", meshPort)
	}

	url := fmt.Sprintf("https://%s:%d/chain/status", host, httpPort)
	c := newInternalHTTPSClient(2 * time.Second)
	resp, err := c.Get(url)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	var st struct {
		Height int64  `json:"height"`
		Tip    string `json:"tip"`
		Hash   string `json:"hash"`
	}
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &st); err != nil {
		return 0, "", err
	}

	tip := st.Tip
	if tip == "" {
		tip = st.Hash
	}
	return st.Height, tip, nil
}

func (m *meshDaemon) requestPeerRange(addr string, from, to int64) ([]Block, error) {

	host, portStr, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return nil, err
	}

	p, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}

	apiPort := p - 1012
	if apiPort <= 0 {
		return nil, fmt.Errorf("bad mesh port %d", p)
	}

	base := fmt.Sprintf("https://%s:%d", host, apiPort)

	out := make([]Block, 0, (to-from)+1)

	for h := from; h <= to; h++ {
		url := fmt.Sprintf("%s/chain/block?h=%d", base, h)

		bs, err := httpGetBytes(url)
		if err != nil {
			return out, err
		}

		var blk Block
		if err := json.Unmarshal(bs, &blk); err != nil {
			return out, err
		}

		out = append(out, blk)
	}

	return out, nil
}

func httpGetBytes(url string) ([]byte, error) {
	client := newInternalHTTPSClient(2 * time.Second)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// reachablePeers returns currently reachable mesh peers (read-only)
func (m *meshDaemon) reachablePeers() []string {
	m.lock.RLock()
	defer m.lock.RUnlock()

	peers := make([]string, 0, len(m.peers))
	for addr, p := range m.peers {
		if p.Reachable {
			peers = append(peers, addr)
		}
	}

	sort.Strings(peers)
	return peers
}

func (m *meshDaemon) reachablePeersForSync() []string {
	now := time.Now()
	m.lock.RLock()
	defer m.lock.RUnlock()

	peers := make([]string, 0, len(m.peers))
	for addr, p := range m.peers {
		if !p.Reachable {
			continue
		}
		if !p.SuppressedUntil.IsZero() && p.SuppressedUntil.After(now) {
			continue
		}
		peers = append(peers, addr)
	}
	sort.Strings(peers)
	return peers
}
