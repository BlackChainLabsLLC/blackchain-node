package mesh

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type ChainSnapshot struct {
	Height       int64              `json:"height"`
	Hash         string             `json:"hash"`
	Accounts     map[string]Account `json:"accounts"`
	AccountsHash string             `json:"accounts_hash"`
	SnapshotHash string             `json:"snapshot_hash"`
}

// snapshotPath returns location for snapshot file.
func (c *ProductionChain) snapshotPath() string {
	base := c.persistDir
	if base == "" {
		base = c.dataDir
	}
	if base == "" {
		base = "data"
	}
	return filepath.Join(base, "snapshot.json")
}

// ExportSnapshotLocked exports current state into a snapshot struct.
// Caller must hold c.mu (RLock/Lock ok; values are copied).
func (c *ProductionChain) ExportSnapshotLocked() ChainSnapshot {
	out := ChainSnapshot{
		Height:   c.height,
		Hash:     c.tip,
		Accounts: make(map[string]Account, len(c.accounts)),
	}
	for k, v := range c.accounts {
		out.Accounts[k] = *v
	}
	out.AccountsHash = computeAccountsHash(c.accounts)
	out.SnapshotHash = computeSnapshotHash(out.Height, out.Hash, out.AccountsHash)
	return out
}

// SaveSnapshotLocked writes snapshot.json atomically.
// Caller must hold c.mu (Lock).
func (c *ProductionChain) SaveSnapshotLocked() error {
	snap := c.ExportSnapshotLocked()
	raw, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	path := c.snapshotPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadSnapshotFromDisk attempts to load snapshot.json and apply it as chain base.
// This resets in-memory state to snapshot, then replays any persisted blocks ABOVE snapshot height.
func (c *ProductionChain) LoadSnapshotFromDisk() (bool, error) {
	path := c.snapshotPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	var snap ChainSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return false, err
	}

	// Reset chain state to snapshot
	c.height = snap.Height
	c.tip = snap.Hash
	c.accounts = make(map[string]*Account, len(snap.Accounts))
	for k, v := range snap.Accounts {
		vv := v
		c.accounts[k] = &vv
	}
	if snap.AccountsHash == "" {
		return false, fmt.Errorf("snapshot missing accounts_hash")
	}
	if computeAccountsHash(c.accounts) != snap.AccountsHash {
		return false, fmt.Errorf("snapshot accounts_hash mismatch")
	}
	if snap.SnapshotHash == "" {
		return false, fmt.Errorf("snapshot missing snapshot_hash")
	}
	if computeSnapshotHash(snap.Height, snap.Hash, snap.AccountsHash) != snap.SnapshotHash {
		return false, fmt.Errorf("snapshot hash mismatch")
	}
	c.blocks = make(map[int64]Block)
	c.pending = make(map[int64]Block)
	c.mempool = make([]Tx, 0, 256)

	// Replay persisted blocks above snapshot height, if any exist.
	dir := c.blockDir()
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}

	type pair struct {
		h int64
		p string
	}
	var files []pair
	for _, e := range ents {
		var h int64
		if _, err := fmt.Sscanf(e.Name(), "%d.json", &h); err == nil && h > snap.Height {
			files = append(files, pair{h: h, p: filepath.Join(dir, e.Name())})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].h < files[j].h })

	for _, f := range files {
		raw, err := os.ReadFile(f.p)
		if err != nil {
			return false, err
		}
		var b Block
		if err := json.Unmarshal(raw, &b); err != nil {
			return false, err
		}
		// Use full validation
		if err := c.applyBlockLocked(b); err != nil {
			return false, fmt.Errorf("replay post-snapshot height %d: %w", b.Height, err)
		}
	}

	return true, nil
}

// LoadSnapshotFromBytes applies a snapshot payload received over the mesh.
// It resets in-memory state to the snapshot base.
func (c *ProductionChain) LoadSnapshotFromBytes(raw []byte) (bool, error) {
	var snap ChainSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return false, err
	}

	// Reset chain state to snapshot
	c.height = snap.Height
	c.tip = snap.Hash

	c.accounts = make(map[string]*Account, len(snap.Accounts))
	for k, v := range snap.Accounts {
		vv := v
		c.accounts[k] = &vv
	}
	if snap.AccountsHash == "" {
		return false, fmt.Errorf("snapshot missing accounts_hash")
	}
	if computeAccountsHash(c.accounts) != snap.AccountsHash {
		return false, fmt.Errorf("snapshot accounts_hash mismatch")
	}
	if snap.SnapshotHash == "" {
		return false, fmt.Errorf("snapshot missing snapshot_hash")
	}
	if computeSnapshotHash(snap.Height, snap.Hash, snap.AccountsHash) != snap.SnapshotHash {
		return false, fmt.Errorf("snapshot hash mismatch")
	}

	c.blocks = make(map[int64]Block)
	c.pending = make(map[int64]Block)
	c.mempool = make([]Tx, 0, 256)

	return true, nil
}
func computeAccountsHash(accts map[string]*Account) string {
	type pair struct {
		Addr  string
		Bal   int64
		Nonce int64
	}
	lst := make([]pair, 0, len(accts))
	for k, v := range accts {
		lst = append(lst, pair{k, v.Balance, v.Nonce})
	}
	sort.Slice(lst, func(i, j int) bool { return lst[i].Addr < lst[j].Addr })
	raw, _ := json.Marshal(lst)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func computeSnapshotHash(height int64, tip string, accountsHash string) string {
	raw := []byte(fmt.Sprintf("%d|%s|%s", height, tip, accountsHash))
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
