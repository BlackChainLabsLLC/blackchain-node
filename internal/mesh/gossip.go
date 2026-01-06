package mesh

import (
	"encoding/json"
	"log"
	"net"
	"strings"
	"time"
)

// gossipOrigin is called when this node originates a broadcast.
// RULE: send ONLY to bootstrapPeers (fixed topology).
func (m *meshDaemon) gossipOrigin(body, topic, id string, ttl int) int {
	if ttl <= 0 {
		return 0
	}

	wm := wireMessage{
		ID:    id,
		Type:  "msg",
		From:  m.id, // origin
		Body:  body,
		Topic: topic,
		Time:  time.Now().Unix(),
		TTL:   ttl,
		Via:   m.id, // immediate hop (sender)
	}

	m.markSeen(id)

	// Topology-driven fanout: bootstrapPeers only.
	sent := 0
	for _, addr := range m.bootstrapPeers {
		addr = strings.TrimSpace(addr)
		if addr == "" || addr == m.id {
			continue
		}
		wm.To = addr
		go sendWireOnce(wm, addr)
		sent++
	}
	return sent
}

// gossipForward forwards a broadcast to neighbors while TTL remains.
// RULES:
// 1) TTL decrements by 1 per hop.
// 2) Forward ONLY to bootstrapPeers (fixed topology).
// 3) Never forward back to the immediate hop (wm.Via).
func (m *meshDaemon) gossipForward(wm wireMessage) {
	ttl := wm.TTL - 1
	if ttl <= 0 {
		return
	}
	wm.TTL = ttl

	// We are the immediate hop for the next receiver.
	prevHop := strings.TrimSpace(wm.Via)
	wm.Via = m.id

	for _, addr := range m.bootstrapPeers {
		addr = strings.TrimSpace(addr)
		if addr == "" || addr == m.id {
			continue
		}
		// Do not send back to who just sent to us.
		if prevHop != "" && addr == prevHop {
			continue
		}
		wm.To = addr
		go sendWireOnce(wm, addr)
	}
}

// sendWireOnce sends a single wireMessage to one neighbor.
func sendWireOnce(wm wireMessage, addr string) {
	data, err := json.Marshal(wm)
	if err != nil {
		log.Printf("[gossip] encode error: %v", err)
		return
	}

	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		log.Printf("[gossip] dial %s error: %v", addr, err)
		return
	}
	defer conn.Close()

	if _, err := conn.Write(append(data, '\n')); err != nil {
		log.Printf("[gossip] write to %s error: %v", addr, err)
		return
	}
}
