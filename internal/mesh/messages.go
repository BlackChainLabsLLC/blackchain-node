package mesh

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
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

	var wm wireMessage
	if err := json.Unmarshal(line, &wm); err != nil || wm.Type != "msg" {
		log.Printf("[handleIncoming] recv raw_from_conn=%s bytes=%d", conn.RemoteAddr().String(), len(line))
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

	if wm.TTL > 0 {
		m.gossipForward(wm)
	}
}
