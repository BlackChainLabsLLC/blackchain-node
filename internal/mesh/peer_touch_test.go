package mesh

import (
	"strings"
	"testing"
)

func TestSanitizeLearnedPeerAddrRejectsWildcardAndSelf(t *testing.T) {
	_, err := sanitizeLearnedPeerAddr("0.0.0.0:7072", "127.0.0.1:7072", map[string]*Peer{})
	if err == nil || !strings.Contains(err.Error(), "wildcard") {
		t.Fatalf("expected wildcard rejection, got %v", err)
	}

	_, err = sanitizeLearnedPeerAddr("localhost:7072", "127.0.0.1:7072", map[string]*Peer{})
	if err == nil || !strings.Contains(err.Error(), "self address") {
		t.Fatalf("expected self-address rejection, got %v", err)
	}
}

func TestSanitizeLearnedPeerAddrDeduplicatesEquivalentLoopback(t *testing.T) {
	existing := map[string]*Peer{
		"127.0.0.1:7072": {Addr: "127.0.0.1:7072"},
	}

	addr, err := sanitizeLearnedPeerAddr("localhost:7072", "127.0.0.1:8080", existing)
	if err != nil {
		t.Fatalf("sanitize learned peer: %v", err)
	}
	if addr != "127.0.0.1:7072" {
		t.Fatalf("expected deduped existing addr, got %s", addr)
	}
}

func TestSanitizeLearnedPeerAddrRejectsMalformedPeer(t *testing.T) {
	_, err := sanitizeLearnedPeerAddr("bad-peer", "", map[string]*Peer{})
	if err == nil || !strings.Contains(err.Error(), "invalid learned peer address") {
		t.Fatalf("expected malformed learned peer rejection, got %v", err)
	}
}
