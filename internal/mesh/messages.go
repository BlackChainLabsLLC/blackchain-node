package mesh

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Message is the high-level inbox entry returned by /inbox.
type Message struct {
	From string    `json:"from"`
	To   string    `json:"to"`
	Body string    `json:"body"`
	Time time.Time `json:"time"`
}

// wireMessage MUST remain compatible with all mesh modules.
type wireMessage struct {
	ID     string `json:"id,omitempty"`
	Type   string `json:"type"`
	From   string `json:"from"`          // origin
	Via    string `json:"via,omitempty"` // immediate hop
	To     string `json:"to,omitempty"`
	Body   string `json:"body"`
	Topic  string `json:"topic,omitempty"`
	Time   int64  `json:"time"`
	TTL    int    `json:"ttl,omitempty"`
	PubKey string `json:"pubkey,omitempty"`
}

// handleIncoming processes all inbound mesh TCP messages.
func (m *meshDaemon) handleIncoming(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil || len(line) == 0 {
		return
	}

	line = bytes.TrimSpace(line)
	log.Printf("[DEBUG] inbound raw from_conn=%s bytes=%d", conn.RemoteAddr().String(), len(line))

	var wm wireMessage
	if err := json.Unmarshal(line, &wm); err != nil {
		// non-wire traffic (e.g., "mesh-hello\n") or garbage: keep old behavior
		log.Printf("[handleIncoming] recv raw_from_conn=%s bytes=%d", conn.RemoteAddr().String(), len(line))
		_, _ = conn.Write([]byte("blackchain-mesh-ok\n"))
		return
	}

	// Only accept known types; everything else treated like non-wire traffic.
	switch wm.Type {
	case "msg", "block":
		// ok
	default:
		log.Printf("[handleIncoming] drop unknown type=%q from_conn=%s bytes=%d", wm.Type, conn.RemoteAddr().String(), len(line))
		_, _ = conn.Write([]byte("blackchain-mesh-ok\n"))
		return
	}

	if wm.ID == "" {
		wm.ID = fmt.Sprintf("auto-%d", time.Now().UnixNano())
	}

	m.lock.RLock()
	_, seen := m.seen[wm.ID]
	m.lock.RUnlock()
	if seen {
		return
	}
	m.markSeen(wm.ID)

	// ACTIVITY TRUTH:
	// Touch ONLY the immediate neighbor (Via). Never the origin.
	sender := strings.TrimSpace(wm.Via)
	if sender != "" {
		m.TouchPeer(sender)
	}

	switch wm.Type {
	case "msg":
		msg := Message{
			From: wm.From,
			To:   wm.To,
			Body: wm.Body,
			Time: time.Unix(wm.Time, 0),
		}

		m.lock.Lock()
		m.inbox = append(m.inbox, msg)
		m.lock.Unlock()

		log.Printf("[msg] received from %s ttl=%d id=%s: %q",
			msg.From, wm.TTL, wm.ID, msg.Body)

		// If this "msg" body is actually SignedStateAnnouncement, trigger catch-up.
		_ = m.maybeSyncFromSignedState(sender, wm.Body)

		if wm.TTL > 0 {
			m.gossipForward(wm)
		}

	case "block":
		var b Block
		if err := json.Unmarshal([]byte(wm.Body), &b); err != nil {
			log.Printf("[block] decode error id=%s from=%s via=%s: %v", wm.ID, wm.From, wm.Via, err)
			return
		}

		var applied bool
		m.chain.mu.Lock()
		applied, err = m.chain.applyBlockOrBufferLocked(b)
		m.chain.mu.Unlock()
		if err != nil {
			log.Printf("[block] apply error height=%d id=%s from=%s via=%s: %v", b.Height, wm.ID, wm.From, wm.Via, err)
			return
		}

		if applied {
			log.Printf("[block] applied height=%d id=%s from=%s via=%s ttl=%d", b.Height, wm.ID, wm.From, wm.Via, wm.TTL)
		} else {
			log.Printf("[block] buffered height=%d id=%s from=%s via=%s ttl=%d", b.Height, wm.ID, wm.From, wm.Via, wm.TTL)
		}

		if wm.TTL > 0 {
			m.gossipForward(wm)
		}
	}
}

// maybeSyncFromSignedState inspects a "msg" body that might actually be a SignedStateAnnouncement.
// If the peer is ahead, it pulls missing blocks from the peer's API and applies them locally.
// Convention: peer mesh port 707x maps to API port 606x (delta -1010).
func (m *meshDaemon) maybeSyncFromSignedState(via string, body string) bool {
	var ssa SignedStateAnnouncement
	if err := json.Unmarshal([]byte(body), &ssa); err != nil {
		return false
	}
	// Basic shape check (avoid false positives)
	if ssa.NodeID == "" || ssa.Height <= 0 {
		return false
	}

	localH := (func() int { m.chain.mu.Lock(); defer m.chain.mu.Unlock(); return int(m.chain.height) })()
	if int(ssa.Height) <= localH {
		return true // recognized signed-state, but we're not behind
	}

	host, portStr, err := net.SplitHostPort(strings.TrimSpace(via))
	if err != nil {
		return true // recognized, but cannot derive peer API
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return true
	}
	apiPort := p - 1010
	if apiPort <= 0 {
		return true
	}
	base := fmt.Sprintf("http://%s:%d", host, apiPort)

	// Pull and apply blocks (Lego-simple: sequential)
	for h := localH + 1; h <= int(ssa.Height); h++ {
		url := fmt.Sprintf("%s/chain/block?h=%d", base, h)
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("[sync] fetch error from=%s url=%s: %v", via, url, err)
			return true
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			log.Printf("[sync] fetch non-200 from=%s url=%s code=%d body=%q", via, url, resp.StatusCode, strings.TrimSpace(string(b)))
			return true
		}

		var blk Block
		if err := json.Unmarshal(b, &blk); err != nil {
			log.Printf("[sync] decode error from=%s h=%d: %v", via, h, err)
			return true
		}

		var applied bool
		m.chain.mu.Lock()
		applied, err = m.chain.applyBlockOrBufferLocked(blk)
		m.chain.mu.Unlock()
		if err != nil {
			log.Printf("[sync] apply error from=%s h=%d: %v", via, h, err)
			return true
		}
		if applied {
			log.Printf("[sync] applied h=%d from=%s", h, via)
		} else {
			log.Printf("[sync] buffered h=%d from=%s", h, via)
		}
	}

	return true
}
