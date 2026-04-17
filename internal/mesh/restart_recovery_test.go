package mesh

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadFromDiskRecoversInterruptedTmpBlock(t *testing.T) {
	c := newTestChain(t)
	b := makeSignedBlockForChain(t, c, 1, "", time.Unix(100, 0))
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		t.Fatalf("marshal block: %v", err)
	}
	tmpPath := filepath.Join(c.blockDir(), "1.json.tmp")
	if err := os.MkdirAll(filepath.Dir(tmpPath), 0o755); err != nil {
		t.Fatalf("mkdir block dir: %v", err)
	}
	if err := os.WriteFile(tmpPath, raw, 0o644); err != nil {
		t.Fatalf("write tmp block: %v", err)
	}

	if err := c.loadFromDisk(); err != nil {
		t.Fatalf("load from disk: %v", err)
	}
	if c.height != 1 {
		t.Fatalf("expected recovered height 1, got %d", c.height)
	}
	if _, err := os.Stat(filepath.Join(c.blockDir(), "1.json")); err != nil {
		t.Fatalf("expected final block after tmp recovery: %v", err)
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("expected tmp block removed after recovery, stat err=%v", err)
	}
}

func TestLoadSnapshotFromDiskRecoversInterruptedTmpSnapshot(t *testing.T) {
	c := newTestChain(t)
	c.mu.Lock()
	c.height = 3
	c.tip = "tip-3"
	c.accounts["alice"] = &Account{Balance: 42, Nonce: 1}
	snap := c.ExportSnapshotLocked()
	c.mu.Unlock()

	raw, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	tmpPath := c.snapshotPath() + ".tmp"
	if err := os.MkdirAll(filepath.Dir(tmpPath), 0o755); err != nil {
		t.Fatalf("mkdir snapshot dir: %v", err)
	}
	if err := os.WriteFile(tmpPath, raw, 0o644); err != nil {
		t.Fatalf("write tmp snapshot: %v", err)
	}

	c2 := newTestChain(t)
	c2.dataDir = c.dataDir
	c2.persistDir = c.persistDir
	ok, err := c2.LoadSnapshotFromDisk()
	if err != nil {
		t.Fatalf("load snapshot from disk: %v", err)
	}
	if !ok || c2.height != 3 || c2.tip != "tip-3" {
		t.Fatalf("expected recovered snapshot height/tip, ok=%v height=%d tip=%s", ok, c2.height, c2.tip)
	}
	if _, err := os.Stat(c.snapshotPath()); err != nil {
		t.Fatalf("expected final snapshot after tmp recovery: %v", err)
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("expected tmp snapshot removed after recovery, stat err=%v", err)
	}
}

func TestApplyBlockLockedPersistFailureDoesNotMutateState(t *testing.T) {
	c := newTestChain(t)
	b := makeSignedBlockForChain(t, c, 1, "", time.Unix(100, 0))
	blockedPath := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blockedPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocked path: %v", err)
	}
	c.persistDir = blockedPath
	beforeHeight := c.height
	beforeTip := c.tip

	if err := c.applyBlockLocked(b); err == nil {
		t.Fatalf("expected persist failure")
	}
	if c.height != beforeHeight || c.tip != beforeTip {
		t.Fatalf("persist failure mutated chain head: height=%d tip=%s", c.height, c.tip)
	}
}

func TestShutdownIsIdempotent(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	runCtx, runCancel := context.WithCancel(context.Background())
	m := &meshDaemon{
		listener:  ln,
		runCtx:    runCtx,
		runCancel: runCancel,
	}

	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("first shutdown: %v", err)
	}
	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
}
