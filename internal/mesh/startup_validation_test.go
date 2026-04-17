package mesh

import "testing"

func TestValidateStartupConfigRejectsSelfPeer(t *testing.T) {
	cfg := &MeshConfig{
		Listen:     "127.0.0.1:7070",
		HttpListen: "127.0.0.1:6060",
		Peers:      []string{"127.0.0.1:7070"},
	}
	if err := validateStartupConfig(cfg); err == nil {
		t.Fatal("expected self-peer validation error")
	}
}

func TestValidateStartupConfigRejectsInvalidPeer(t *testing.T) {
	cfg := &MeshConfig{
		Listen:     "127.0.0.1:7070",
		HttpListen: "127.0.0.1:6060",
		Peers:      []string{"bad-peer"},
	}
	if err := validateStartupConfig(cfg); err == nil {
		t.Fatal("expected invalid peer validation error")
	}
}

func TestValidateStartupConfigAcceptsValidConfig(t *testing.T) {
	cfg := &MeshConfig{
		Listen:     "127.0.0.1:7070",
		HttpListen: "127.0.0.1:6060",
		Peers:      []string{"127.0.0.1:7071", "127.0.0.1:7072"},
	}
	if err := validateStartupConfig(cfg); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestNormalizeBootstrapPeersDeduplicatesAndSorts(t *testing.T) {
	peers := []string{"127.0.0.1:7072", "127.0.0.1:7071", "127.0.0.1:7072"}
	got, err := normalizeBootstrapPeers(peers, "127.0.0.1:7070")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if len(got) != 2 || got[0] != "127.0.0.1:7071" || got[1] != "127.0.0.1:7072" {
		t.Fatalf("unexpected normalized peers: %#v", got)
	}
}
