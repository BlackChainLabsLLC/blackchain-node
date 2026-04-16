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

/*
BOOTSTRAP SYNC
- Poll reachable peers
- Ask heights
- Pick highest
- Pull blocks
- Apply
*/

func (m *meshDaemon) bootstrapSync(ctx context.Context) {

	// Gate chain sync until at least one peer is reachable
	for {
		peers := m.reachablePeers()
		if len(peers) > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	log.Println("[sync] reachable peers detected, starting sync")
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[sync] panic: %v", r)
		}
	}()

	for {

		peers := m.reachablePeers()

		if len(peers) == 0 {
			time.Sleep(300 * time.Millisecond)
			continue
		}

		log.Printf("[sync] probing peers: %v", peers)

		type cand struct {
			Addr   string
			Height int64
			Tip    string
		}

		var cands []cand

		for _, addr := range m.reachablePeers() {
			h, tip, err := m.requestPeerHeight(addr)
			if err != nil {
				log.Printf("[sync] height err %s: %v", addr, err)
				continue
			}
			cands = append(cands, cand{addr, h, tip})
		}

		if len(cands) == 0 {
			time.Sleep(300 * time.Millisecond)
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
			log.Printf("[sync] up-to-date local=%d best=%d", localH, best.Height)
			time.Sleep(1 * time.Second)
			continue
		}

		log.Printf("[sync] behind local=%d best=%d @ %s", localH, best.Height, best.Addr)

		const chunk int64 = 50

		for cur := localH + 1; cur <= best.Height; cur += chunk {

			end := cur + chunk - 1
			if end > best.Height {
				end = best.Height
			}

			blocks, err := m.requestPeerRange(best.Addr, cur, end)
			if err != nil {
				log.Printf("[sync] range err %v", err)
				time.Sleep(750 * time.Millisecond)
				continue
			}

			m.chain.mu.Lock()
			for _, b := range blocks {
				_, _ = m.chain.applyBlockOrBufferLocked(b)
			}
			_ = m.chain.drainPendingLocked()
			m.chain.mu.Unlock()
		}

		m.chain.mu.RLock()
		h := m.chain.height
		t := m.chain.tip
		m.chain.mu.RUnlock()

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
