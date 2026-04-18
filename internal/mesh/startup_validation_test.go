package mesh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeBootstrapConfigForTest(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bootstrap.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write bootstrap config: %v", err)
	}
	return path
}

func TestLoadBootstrapPeersFromFileRejectsMalformedJSON(t *testing.T) {
	path := writeBootstrapConfigForTest(t, `{"bootstrap_peers":["127.0.0.1:7072"`)

	_, err := loadBootstrapPeersFromFile(path, nil)
	if err == nil || !strings.Contains(err.Error(), "parse bootstrap config") {
		t.Fatalf("expected malformed bootstrap parse error, got %v", err)
	}
}

func TestLoadBootstrapPeersFromFileRejectsDuplicatePeer(t *testing.T) {
	path := writeBootstrapConfigForTest(t, `{"bootstrap_peers":["127.0.0.1:7072","127.0.0.1:7072"]}`)

	_, err := loadBootstrapPeersFromFile(path, nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate bootstrap peer") {
		t.Fatalf("expected duplicate bootstrap rejection, got %v", err)
	}
}

func TestLoadBootstrapPeersFromFileRejectsSelfPeer(t *testing.T) {
	path := writeBootstrapConfigForTest(t, `{"bootstrap_peers":["127.0.0.1:7072"]}`)

	_, err := loadBootstrapPeersFromFile(path, map[string]struct{}{"127.0.0.1:7072": {}})
	if err == nil || !strings.Contains(err.Error(), "self address") {
		t.Fatalf("expected self bootstrap rejection, got %v", err)
	}
}

func TestLoadBootstrapPeersFromFileMissingIsAllowed(t *testing.T) {
	peers, err := loadBootstrapPeersFromFile(filepath.Join(t.TempDir(), "missing.json"), nil)
	if err != nil {
		t.Fatalf("expected missing bootstrap config to be allowed, got %v", err)
	}
	if len(peers) != 0 {
		t.Fatalf("expected no peers for missing bootstrap config, got %v", peers)
	}
}

func TestBuildStartupPeerSetNormalizesAndDeduplicates(t *testing.T) {
	peerSet, err := buildStartupPeerSet(
		[]string{"127.0.0.1:7072", "localhost:7073"},
		[]string{"127.0.0.1:7072", "127.0.0.1:7074"},
	)
	if err != nil {
		t.Fatalf("build startup peer set: %v", err)
	}
	if len(peerSet) != 3 {
		t.Fatalf("expected 3 unique peers, got %d: %v", len(peerSet), peerSet)
	}
	if _, ok := peerSet["127.0.0.1:7072"]; !ok {
		t.Fatalf("expected normalized peer 127.0.0.1:7072")
	}
	if _, ok := peerSet["localhost:7073"]; !ok {
		t.Fatalf("expected peer localhost:7073")
	}
	if _, ok := peerSet["127.0.0.1:7074"]; !ok {
		t.Fatalf("expected peer 127.0.0.1:7074")
	}
}

func TestBuildStartupPeerSetRejectsMalformedPeer(t *testing.T) {
	_, err := buildStartupPeerSet(nil, []string{"bad-peer"})
	if err == nil || !strings.Contains(err.Error(), "invalid startup peer address") {
		t.Fatalf("expected malformed startup peer rejection, got %v", err)
	}
}
