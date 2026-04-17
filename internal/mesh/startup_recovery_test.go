package mesh

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func makeValidBlock(t *testing.T, c *ProductionChain) Block {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pubHex := hex.EncodeToString(pub)
	b := Block{
		Producer:       pubHex,
		ProducerAddr:   "producer",
		ValidatorID:    pubHex,
		Height:         c.height + 1,
		PrevHash:       c.tip,
		Reward:         blockRewardForHeight(c.height + 1),
		TimeUTC:        time.Unix(1700000000, 0).UTC(),
		Txs:            nil,
		ProducerPubKey: pubHex,
	}
	b.BalancesRoot = c.calcStateHashWithBlock(b)
	b.Hash = c.calcBlockHash(b)
	SignBlock(&b, priv)
	return b
}

func writeBlockJSON(t *testing.T, root string, fileHeight int64, b Block) {
	t.Helper()
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal block: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "blocks"), 0o755); err != nil {
		t.Fatalf("mkdir blocks: %v", err)
	}
	path := filepath.Join(root, "blocks", strconv.FormatInt(fileHeight, 10)+".json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write block file: %v", err)
	}
}

func TestRecoverFromDiskModes(t *testing.T) {
	t.Run("clean_start", func(t *testing.T) {
		root := t.TempDir()
		c := newProductionChain()
		c.persistDir = root
		c.ensureGenesisLocked()

		report, err := c.RecoverFromDisk()
		if err != nil {
			t.Fatalf("recover: %v", err)
		}
		if report.Mode != StartupModeCleanStart {
			t.Fatalf("mode got=%s", report.Mode)
		}
	})

	t.Run("replay_start", func(t *testing.T) {
		root := t.TempDir()
		c := newProductionChain()
		c.persistDir = root
		c.ensureGenesisLocked()
		b := makeValidBlock(t, c)
		writeBlockJSON(t, root, 1, b)

		report, err := c.RecoverFromDisk()
		if err != nil {
			t.Fatalf("recover: %v", err)
		}
		if report.Mode != StartupModeReplayStart {
			t.Fatalf("mode got=%s", report.Mode)
		}
		if c.height != 1 {
			t.Fatalf("height got=%d", c.height)
		}
	})

	t.Run("snapshot_restore", func(t *testing.T) {
		root := t.TempDir()
		c := newProductionChain()
		c.persistDir = root
		c.height = 3
		c.tip = "abc"
		c.accounts["alice"] = &Account{Balance: 5, Nonce: 2}
		snap := c.ExportSnapshotLocked()
		raw, _ := json.Marshal(snap)
		if err := os.WriteFile(filepath.Join(root, "snapshot.json"), raw, 0o644); err != nil {
			t.Fatalf("write snapshot: %v", err)
		}

		fresh := newProductionChain()
		fresh.persistDir = root
		fresh.ensureGenesisLocked()
		report, err := fresh.RecoverFromDisk()
		if err != nil {
			t.Fatalf("recover: %v", err)
		}
		if report.Mode != StartupModeSnapshotRestore {
			t.Fatalf("mode got=%s", report.Mode)
		}
		if fresh.height != 3 {
			t.Fatalf("height got=%d", fresh.height)
		}
	})
}

func TestRecoverFromDiskCorruptionQuarantine(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "snapshot.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatalf("write bad snapshot: %v", err)
	}
	c := newProductionChain()
	c.persistDir = root
	c.ensureGenesisLocked()

	report, err := c.RecoverFromDisk()
	if err == nil {
		t.Fatalf("expected error")
	}
	if report.Mode != StartupModeCorruptionHalt {
		t.Fatalf("mode got=%s", report.Mode)
	}
	if report.QuarantineDir == "" {
		t.Fatalf("expected quarantine dir")
	}
	if _, err := os.Stat(filepath.Join(root, "STARTUP_HALT")); err != nil {
		t.Fatalf("halt marker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(report.QuarantineDir, "snapshot.json")); err != nil {
		t.Fatalf("quarantined snapshot: %v", err)
	}
}

func TestRecoverFromDiskCorruptionReplayMismatch(t *testing.T) {
	root := t.TempDir()
	c := newProductionChain()
	c.persistDir = root
	c.ensureGenesisLocked()
	b := makeValidBlock(t, c)
	writeBlockJSON(t, root, 2, b)

	report, err := c.RecoverFromDisk()
	if err == nil {
		t.Fatalf("expected mismatch error")
	}
	if report.Mode != StartupModeCorruptionHalt {
		t.Fatalf("mode got=%s", report.Mode)
	}
}
