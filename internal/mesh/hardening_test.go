package mesh

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestChain(t *testing.T) *ProductionChain {
	t.Helper()
	dir := t.TempDir()
	c := newProductionChain()
	c.dataDir = dir
	c.persistDir = dir
	c.ensureGenesisLocked()
	return c
}

func makeSignedBlockForChain(t *testing.T, c *ProductionChain, height int64, prevHash string, at time.Time) Block {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()

	pubHex := c.ValidatorIDLocked()
	privHex, err := c.ValidatorPrivHexLocked()
	if err != nil {
		t.Fatalf("validator private key: %v", err)
	}
	privRaw, err := hex.DecodeString(privHex)
	if err != nil {
		t.Fatalf("decode validator private key: %v", err)
	}

	b := Block{
		ProducerAddr: "wallet-test",
		Producer:     pubHex,
		ValidatorID:  pubHex,
		Height:       height,
		PrevHash:     prevHash,
		TimeUTC:      at.UTC(),
		Reward:       blockRewardForHeight(height),
	}
	b.BalancesRoot = c.calcStateHashWithBlock(b)
	b.Hash = c.calcBlockHash(b)
	SignBlock(&b, ed25519.PrivateKey(privRaw))
	return b
}

func TestLoadFromDiskCorruptBlockQuarantinesAndHalts(t *testing.T) {
	c := newTestChain(t)
	blockPath := filepath.Join(c.blockDir(), "1.json")
	if err := os.MkdirAll(filepath.Dir(blockPath), 0o755); err != nil {
		t.Fatalf("mkdir block dir: %v", err)
	}
	if err := os.WriteFile(blockPath, []byte(`{"height":1`), 0o644); err != nil {
		t.Fatalf("write corrupt block: %v", err)
	}

	err := c.loadFromDisk()
	if err == nil || !strings.Contains(err.Error(), "startup halted for operator action") {
		t.Fatalf("expected startup-halted corruption error, got %v", err)
	}
	if _, statErr := os.Stat(blockPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected corrupt block to be quarantined, stat err=%v", statErr)
	}
	entries, err := os.ReadDir(filepath.Join(c.blockDir(), "quarantine"))
	if err != nil {
		t.Fatalf("read quarantine dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one quarantined file, got %d", len(entries))
	}
}

func TestLoadFromDiskSemanticReplayFailureQuarantinesBlock(t *testing.T) {
	c := newTestChain(t)
	b := makeSignedBlockForChain(t, c, 1, "", time.Unix(100, 0))
	b.Hash = "bad-hash"

	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		t.Fatalf("marshal invalid block: %v", err)
	}
	blockPath := filepath.Join(c.blockDir(), "1.json")
	if err := os.MkdirAll(filepath.Dir(blockPath), 0o755); err != nil {
		t.Fatalf("mkdir block dir: %v", err)
	}
	if err := os.WriteFile(blockPath, raw, 0o644); err != nil {
		t.Fatalf("write invalid block: %v", err)
	}

	err = c.loadFromDisk()
	if err == nil || !strings.Contains(err.Error(), "startup halted for operator action") {
		t.Fatalf("expected semantic replay corruption error, got %v", err)
	}
	if _, statErr := os.Stat(blockPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected invalid replay block to be quarantined, stat err=%v", statErr)
	}
}

func TestLoadSnapshotFromBytesValidationFailureDoesNotMutateState(t *testing.T) {
	c := newTestChain(t)
	c.mu.Lock()
	c.height = 7
	c.tip = "tip-before"
	c.accounts["alice"] = &Account{Balance: 42, Nonce: 3}
	beforeHeight := c.height
	beforeTip := c.tip
	beforeBal := c.accounts["alice"].Balance
	snap := c.ExportSnapshotLocked()
	c.mu.Unlock()

	snap.SnapshotHash = "bad-snapshot-hash"
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}

	c.mu.Lock()
	ok, err := c.LoadSnapshotFromBytes(raw)
	c.mu.Unlock()
	if err == nil {
		t.Fatalf("expected snapshot validation error")
	}
	if ok {
		t.Fatalf("expected snapshot apply to fail")
	}
	if c.height != beforeHeight || c.tip != beforeTip {
		t.Fatalf("snapshot failure mutated chain head: height=%d tip=%s", c.height, c.tip)
	}
	if got := c.accounts["alice"].Balance; got != beforeBal {
		t.Fatalf("snapshot failure mutated account balance: got=%d want=%d", got, beforeBal)
	}
}

func TestApplyBlockOrBufferRejectsConflictingStaleAndPending(t *testing.T) {
	c := newTestChain(t)

	b1 := makeSignedBlockForChain(t, c, 1, "", time.Unix(100, 0))
	c.mu.Lock()
	applied, err := c.applyBlockOrBufferLocked(b1)
	c.mu.Unlock()
	if err != nil || !applied {
		t.Fatalf("apply initial block: applied=%v err=%v", applied, err)
	}

	c.mu.Lock()
	applied, err = c.applyBlockOrBufferLocked(b1)
	c.mu.Unlock()
	if err == nil || applied {
		t.Fatalf("duplicate block should be rejected, applied=%v err=%v", applied, err)
	}
	if !strings.Contains(err.Error(), "duplicate committed proposal") {
		t.Fatalf("expected duplicate committed proposal rejection, got %v", err)
	}

	conflictStale := makeSignedBlockForChain(t, c, 1, "", time.Unix(101, 0))
	c.mu.Lock()
	_, err = c.applyBlockOrBufferLocked(conflictStale)
	c.mu.Unlock()
	if err == nil || !strings.Contains(err.Error(), "conflicting committed proposal") {
		t.Fatalf("expected conflicting committed proposal rejection, got %v", err)
	}

	pendingA := makeSignedBlockForChain(t, c, 3, "future-prev-a", time.Unix(200, 0))
	c.mu.Lock()
	applied, err = c.applyBlockOrBufferLocked(pendingA)
	c.mu.Unlock()
	if err != nil || applied {
		t.Fatalf("expected future block to buffer, applied=%v err=%v", applied, err)
	}

	pendingB := makeSignedBlockForChain(t, c, 3, "future-prev-b", time.Unix(201, 0))
	c.mu.Lock()
	_, err = c.applyBlockOrBufferLocked(pendingB)
	c.mu.Unlock()
	if err == nil || !strings.Contains(err.Error(), "conflicting pending proposal") {
		t.Fatalf("expected conflicting pending proposal rejection, got %v", err)
	}
}

func TestTouchReachableSuppresssFlappingPeer(t *testing.T) {
	m := &meshDaemon{
		peers: make(map[string]*Peer),
	}
	addr := "127.0.0.1:9001"

	m.TouchReachable(addr, false)
	m.TouchReachable(addr, false)
	if !m.shouldDialPeer(addr, time.Now()) {
		t.Fatalf("peer should still be dialable before suppression threshold")
	}

	m.TouchReachable(addr, false)
	if m.shouldDialPeer(addr, time.Now()) {
		t.Fatalf("peer should be temporarily suppressed after repeated failures")
	}

	m.lock.RLock()
	suppressedUntil := m.peers[addr].SuppressedUntil
	m.lock.RUnlock()
	if suppressedUntil.IsZero() {
		t.Fatalf("expected suppressed-until to be set")
	}
	if !m.shouldDialPeer(addr, suppressedUntil.Add(time.Second)) {
		t.Fatalf("peer should become dialable after suppression window")
	}

	m.TouchReachable(addr, true)
	if !m.shouldDialPeer(addr, time.Now()) {
		t.Fatalf("successful reachability should clear suppression")
	}
}
