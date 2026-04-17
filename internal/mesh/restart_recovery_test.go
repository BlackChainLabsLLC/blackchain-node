package mesh

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mustNextBlock(t *testing.T, c *ProductionChain, priv ed25519.PrivateKey, pubHex string) Block {
	t.Helper()
	b := Block{
		Producer:     pubHex,
		ValidatorID:  pubHex,
		Height:       c.height + 1,
		PrevHash:     c.tip,
		TimeUTC:      time.Now().UTC(),
		Reward:       blockRewardForHeight(c.height + 1),
		Txs:          nil,
		ProducerAddr: "",
	}
	b.BalancesRoot = c.calcStateHashWithBlock(b)
	b.Hash = c.calcBlockHash(b)
	SignBlock(&b, priv)
	return b
}

func TestLoadFromDiskRecoversInterruptedTmpBlock(t *testing.T) {
	tmp := t.TempDir()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pubHex := hex.EncodeToString(pub)

	c := newProductionChain()
	c.persistDir = tmp
	c.ensureGenesisLocked()

	b1 := mustNextBlock(t, c, priv, pubHex)
	if err := c.applyBlockLocked(b1); err != nil {
		t.Fatalf("apply b1: %v", err)
	}

	b2 := mustNextBlock(t, c, priv, pubHex)
	raw2, err := json.MarshalIndent(b2, "", "  ")
	if err != nil {
		t.Fatalf("marshal b2: %v", err)
	}
	path2 := filepath.Join(tmp, "blocks", "2.json")
	if err := os.WriteFile(path2+".tmp", raw2, 0o644); err != nil {
		t.Fatalf("write tmp b2: %v", err)
	}

	r := newProductionChain()
	r.persistDir = tmp
	r.ensureGenesisLocked()
	if err := r.loadFromDisk(); err != nil {
		t.Fatalf("loadFromDisk: %v", err)
	}
	if r.height != 2 {
		t.Fatalf("recovered height = %d, want 2", r.height)
	}
	if _, err := os.Stat(path2); err != nil {
		t.Fatalf("expected recovered final block file: %v", err)
	}
}

func TestApplyBlockLockedFailsClosedOnPersistError(t *testing.T) {
	tmp := t.TempDir()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pubHex := hex.EncodeToString(pub)

	c := newProductionChain()
	c.persistDir = tmp
	c.ensureGenesisLocked()

	// Poison the blocks path so persist cannot create the directory.
	if err := os.WriteFile(filepath.Join(tmp, "blocks"), []byte("not-a-dir"), 0o644); err != nil {
		t.Fatalf("poison blocks path: %v", err)
	}

	b := mustNextBlock(t, c, priv, pubHex)
	if err := c.applyBlockLocked(b); err == nil {
		t.Fatalf("apply expected persist failure")
	}
	if c.height != 0 {
		t.Fatalf("height mutated on persist failure: %d", c.height)
	}
	if _, ok := c.blocks[1]; ok {
		t.Fatalf("block map mutated on persist failure")
	}
}

func TestLoadSnapshotFromDiskInvalidDoesNotMutateState(t *testing.T) {
	tmp := t.TempDir()
	c := newProductionChain()
	c.persistDir = tmp
	c.height = 7
	c.tip = "tip-before"
	c.accounts["alice"] = &Account{Balance: 50, Nonce: 2}

	bad := ChainSnapshot{
		Height:       10,
		Hash:         "tip-after",
		Accounts:     map[string]Account{"alice": {Balance: 1, Nonce: 1}},
		AccountsHash: "bad-hash",
		SnapshotHash: "bad-snap-hash",
	}
	raw, err := json.Marshal(bad)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "snapshot.json"), raw, 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	ok, err := c.LoadSnapshotFromDisk()
	if err == nil || ok {
		t.Fatalf("expected invalid snapshot failure, got ok=%v err=%v", ok, err)
	}
	if c.height != 7 || c.tip != "tip-before" {
		t.Fatalf("state mutated on bad snapshot: height=%d tip=%q", c.height, c.tip)
	}
	if got := c.accounts["alice"]; got == nil || got.Balance != 50 || got.Nonce != 2 {
		t.Fatalf("accounts mutated on bad snapshot: %+v", got)
	}
}

func TestShutdownIsIdempotent(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	m := &meshDaemon{
		listener: ln,
		nodeID:   "n1",
		runCtx:   context.Background(),
	}

	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("first shutdown: %v", err)
	}
	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown should be no-op: %v", err)
	}
}
