package mesh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	syncRetryBaseDelay = 300 * time.Millisecond
	syncRetryMaxDelay  = 5 * time.Second
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
	failures := 0

	// Gate chain sync until at least one peer is reachable
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		peers := m.reachablePeersForSync(time.Now())
		if len(peers) > 0 {
			failures = 0
			break
		}
		failures++
		time.Sleep(nextRetryDelay(failures))
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

		peers := m.reachablePeersForSync(time.Now())

		if len(peers) == 0 {
			failures++
			time.Sleep(nextRetryDelay(failures))
			continue
		}
		failures = 0

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
				m.recordSyncError(err)
				suppressFor := m.noteSyncFailure(addr, fmt.Errorf("height probe: %w", err))
				log.Printf("[sync] height err %s: %v (suppressed_for=%s)", addr, err, suppressFor)
				continue
			}
			if err := m.notePeerHeightSample(addr, h, tip); err != nil {
				m.recordSyncError(err)
				suppressFor := m.noteSyncFailure(addr, err)
				log.Printf("[sync] rejected peer sample %s: %v (suppressed_for=%s)", addr, err, suppressFor)
				continue
			}
			cands = append(cands, cand{addr, h, tip})
			m.noteSyncSuccess(addr, h, tip)
		}

		if len(cands) == 0 {
			failures++
			time.Sleep(nextRetryDelay(failures))
			continue
		}
		failures = 0

		sort.Slice(cands, func(i, j int) bool {
			return cands[i].Height > cands[j].Height
		})

		best := cands[0]

		m.chain.mu.RLock()
		localH := m.chain.height
		m.chain.mu.RUnlock()
		m.recordSyncHeights(localH, best.Height)

		if best.Height <= localH {
			log.Printf("[sync] up-to-date local=%d best=%d", localH, best.Height)
			time.Sleep(1 * time.Second)
			continue
		}

		log.Printf("[sync] behind local=%d best=%d @ %s", localH, best.Height, best.Addr)

		const chunk int64 = 50

		progressed := false
		for cur := localH + 1; cur <= best.Height; cur += chunk {

			end := cur + chunk - 1
			if end > best.Height {
				end = best.Height
			}

			blocks, err := m.requestPeerRange(best.Addr, cur, end)
			if err != nil {
				m.recordSyncError(err)
				suppressFor := m.noteSyncFailure(best.Addr, fmt.Errorf("range fetch %d-%d: %w", cur, end, err))
				log.Printf("[sync] range err peer=%s from=%d to=%d: %v (suppressed_for=%s)", best.Addr, cur, end, err, suppressFor)
				time.Sleep(nextRetryDelay(failures + 1))
				break
			}

			m.chain.mu.Lock()
			before := m.chain.height
			for _, b := range blocks {
				_, _ = m.chain.applyBlockOrBufferLocked(b)
			}
			_ = m.chain.drainPendingLocked()
			after := m.chain.height
			m.chain.mu.Unlock()
			if after > before {
				progressed = true
			}
		}

		m.chain.mu.RLock()
		h := m.chain.height
		t := m.chain.tip
		m.chain.mu.RUnlock()
		if h < best.Height && !progressed {
			err := fmt.Errorf("sync made no progress from peer=%s local=%d peer=%d", best.Addr, h, best.Height)
			m.recordSyncError(err)
			suppressFor := m.noteSyncFailure(best.Addr, err)
			log.Printf("[sync] no progress from %s (suppressed_for=%s)", best.Addr, suppressFor)
			time.Sleep(nextRetryDelay(failures + 1))
			continue
		}
		m.noteSyncSuccess(best.Addr, h, t)

		log.Printf("[sync] done height=%d tip=%s", h, t)
		// NOTE: do not return; keep syncing for future blocks
		time.Sleep(750 * time.Millisecond)
		continue
	}
}

func (m *meshDaemon) requestPeerHeight(addr string) (int64, string, error) {
	base, err := m.peerAPIBase(addr)
	if err != nil {
		return 0, "", err
	}
	url := base + "/chain/status"
	c, err := newInternalHTTPSClient(2*time.Second, m.dataDir, m.tlsCfg)
	if err != nil {
		return 0, "", err
	}
	resp, err := c.Get(url)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, "", fmt.Errorf("peer status http %d", resp.StatusCode)
	}

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
	if st.Height < 0 {
		return 0, "", fmt.Errorf("peer reported negative height %d", st.Height)
	}
	if st.Height > 0 && strings.TrimSpace(tip) == "" {
		return 0, "", fmt.Errorf("peer reported height %d without tip/hash", st.Height)
	}
	return st.Height, tip, nil
}

func (m *meshDaemon) requestPeerRange(addr string, from, to int64) ([]Block, error) {
	base, err := m.peerAPIBase(addr)
	if err != nil {
		return nil, err
	}

	out := make([]Block, 0, (to-from)+1)

	for h := from; h <= to; h++ {
		url := fmt.Sprintf("%s/chain/block?h=%d", base, h)

		bs, err := m.httpGetBytes(url)
		if err != nil {
			return out, err
		}

		var blk Block
		if err := json.Unmarshal(bs, &blk); err != nil {
			return out, err
		}
		if blk.Height != h {
			return out, fmt.Errorf("peer returned mismatched block height have=%d want=%d", blk.Height, h)
		}

		out = append(out, blk)
	}

	return out, nil
}

func (m *meshDaemon) peerAPIBase(addr string) (string, error) {
	normalized, err := validateTCPAddress("peer", strings.TrimSpace(addr))
	if err != nil {
		return "", err
	}
	if apiAddr, ok := m.peerAPI[normalized]; ok && strings.TrimSpace(apiAddr) != "" {
		return "https://" + strings.TrimSpace(apiAddr), nil
	}

	host, portStr, err := net.SplitHostPort(normalized)
	if err != nil {
		return "", err
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return "", err
	}
	apiPort := p - 1012
	if apiPort <= 0 {
		return "", fmt.Errorf("peer api resolution failed for %s: no explicit peer_api mapping and derived bad http port from %d", addr, p)
	}
	return fmt.Sprintf("https://%s:%d", host, apiPort), nil
}

func (m *meshDaemon) httpGetBytes(url string) ([]byte, error) {
	client, err := newInternalHTTPSClient(2*time.Second, m.dataDir, m.tlsCfg)
	if err != nil {
		return nil, err
	}
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

func (m *meshDaemon) reachablePeersForSync(now time.Time) []string {
	m.lock.RLock()
	defer m.lock.RUnlock()

	peers := make([]string, 0, len(m.peers))
	for addr, p := range m.peers {
		if !p.Reachable {
			continue
		}
		if !p.SuppressedUntil.IsZero() && now.Before(p.SuppressedUntil) {
			continue
		}
		peers = append(peers, addr)
	}

	sort.Strings(peers)
	return peers
}

func (m *meshDaemon) notePeerHeightSample(addr string, height int64, tip string) error {
	addr = strings.TrimSpace(addr)
	tip = strings.TrimSpace(tip)
	if addr == "" {
		return fmt.Errorf("empty peer addr")
	}
	if height < 0 {
		return fmt.Errorf("peer %s reported negative height %d", addr, height)
	}
	if height > 0 && tip == "" {
		return fmt.Errorf("peer %s reported height %d without tip", addr, height)
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	p, ok := m.peers[addr]
	if !ok {
		p = &Peer{Addr: addr}
		m.peers[addr] = p
	}
	if p.LastHeight > 0 {
		if height == p.LastHeight && tip != "" && p.LastTip != "" && p.LastTip != tip {
			return fmt.Errorf("peer %s reported conflicting tip at height=%d old=%s new=%s", addr, height, p.LastTip, tip)
		}
		if height < p.LastHeight {
			return fmt.Errorf("peer %s reported stale height=%d last_height=%d", addr, height, p.LastHeight)
		}
	}
	p.LastHeight = height
	p.LastTip = tip
	return nil
}

func nextRetryDelay(failures int) time.Duration {
	if failures <= 0 {
		return syncRetryBaseDelay
	}
	delay := time.Duration(failures) * syncRetryBaseDelay
	if delay > syncRetryMaxDelay {
		delay = syncRetryMaxDelay
	}
	return delay
}
