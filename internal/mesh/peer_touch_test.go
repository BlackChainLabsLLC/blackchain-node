package mesh

import "testing"

func TestTouchPeerRejectsInvalidAddr(t *testing.T) {
	m := &meshDaemon{peers: map[string]*Peer{}}
	m.TouchPeer("bad-addr")
	if len(m.peers) != 0 {
		t.Fatalf("expected no peers, got %d", len(m.peers))
	}
}

func TestTouchPeerDeduplicatesEquivalentLoopback(t *testing.T) {
	m := &meshDaemon{peers: map[string]*Peer{
		"127.0.0.1:7072": {Addr: "127.0.0.1:7072"},
	}}
	m.TouchPeer("localhost:7072")
	if len(m.peers) != 1 {
		t.Fatalf("expected one deduplicated peer, got %d", len(m.peers))
	}
	if !m.peers["127.0.0.1:7072"].Connected {
		t.Fatalf("expected existing peer to be marked connected")
	}
}
