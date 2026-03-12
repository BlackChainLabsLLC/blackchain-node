package mesh

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"
)

// gossipOrigin is called when this node originates a broadcast.
func (m *meshDaemon) gossipOrigin(body, topic, id string, ttl int) int {
	if ttl <= 0 {
		return 0
	}

	wm := wireMessage{
		ID:    id,
		Type:  "msg",
		From:  m.id,
		Body:  body,
		Topic: topic,
		Time:  time.Now().Unix(),
		TTL:   ttl,
		Via:   m.id,
	}

	m.markSeen(id)

	sent := 0
	for _, addr := range m.bootstrapPeers {
		addr = strings.TrimSpace(addr)
		if addr == "" || addr == m.id {
			continue
		}
		wm.To = addr
		go m.sendWireOnce(wm, addr)
		sent++
	}
	return sent
}

// gossipBlock originates a block broadcast.
func (m *meshDaemon) gossipBlock(b Block, ttl int) int {
	if ttl <= 0 {
		return 0
	}

	raw, err := json.Marshal(b)
	if err != nil {
		return 0
	}

	id := fmtBlockID(m.id, b.Height)

	wm := wireMessage{
		ID:    id,
		Type:  "block",
		From:  m.id,
		Body:  string(raw),
		Topic: "block",
		Time:  time.Now().Unix(),
		TTL:   ttl,
		Via:   m.id,
	}

	m.markSeen(id)

	sent := 0
	for _, addr := range m.bootstrapPeers {
		addr = strings.TrimSpace(addr)
		if addr == "" || addr == m.id {
			continue
		}
		wm.To = addr
		go m.sendWireOnce(wm, addr)
		sent++
	}
	return sent
}

func (m *meshDaemon) gossipForward(wm wireMessage) {
	ttl := wm.TTL - 1
	if ttl <= 0 {
		return
	}
	wm.TTL = ttl

	prevHop := strings.TrimSpace(wm.Via)
	wm.Via = m.id

	for _, addr := range m.bootstrapPeers {
		addr = strings.TrimSpace(addr)
		if addr == "" || addr == m.id {
			continue
		}
		if prevHop != "" && addr == prevHop {
			continue
		}
		wm.To = addr
		go m.sendWireOnce(wm, addr)
	}
}

// TLS-integrated wire sender
func (m *meshDaemon) sendWireOnce(wm wireMessage, addr string) {
	data, err := json.Marshal(wm)
	if err != nil {
		log.Printf("[gossip] encode error: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := meshDialTimeout(ctx, addr, 3*time.Second, m.tlsCfg)
	if err != nil {
		log.Printf("[gossip] dial %s error: %v", addr, err)
		return
	}
	defer conn.Close()

	if _, err := conn.Write(append(data, '\n')); err != nil {
		log.Printf("[gossip] write error: %v", err)
	}
}

// ---- restored helpers (original logic preserved) ----

func fmtBlockID(node string, height int64) string {
	node = strings.ReplaceAll(strings.TrimSpace(node), ":", "_")
	node = strings.ReplaceAll(node, "/", "_")
	return "blk-" + node + "-" +
		time.Now().UTC().Format("20060102T150405.000000000Z") +
		"-" + itoa64(height)
}

func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [32]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + (v % 10))
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
