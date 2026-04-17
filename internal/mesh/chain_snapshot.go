package mesh

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	return writeFileAtomicDurable(path, tmp, raw, 0o644)
}

// LoadSnapshotFromDisk attempts to load snapshot.json and apply it as chain base.
// This resets in-memory state to snapshot, then replays any persisted blocks ABOVE snapshot height.
func (c *ProductionChain) LoadSnapshotFromDisk() (bool, error) {
	exactBlockJSONNameRE := regexp.MustCompile(`^[0-9]+\.json$`)
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
		return false, handleReplayCorruption("snapshot replay", path, fmt.Errorf("decode snapshot: %w", err), false)
	}
	accounts, err := validateSnapshotPayload(raw, snap)
	if err != nil {
		return false, handleReplayCorruption("snapshot replay", path, err, false)
	}

	// Reset chain state to validated snapshot
	c.height = snap.Height
	c.tip = snap.Hash
	c.accounts = accounts
	c.blocks = make(map[int64]Block)
	c.pending = make(map[int64]Block)
	c.resetMempoolLocked(MaxMempoolTxs)

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
		name := e.Name()
		if !exactBlockJSONNameRE.MatchString(name) {
			continue
		}
		var h int64
		if _, err := fmt.Sscanf(name, "%d.json", &h); err == nil && h > snap.Height {
			files = append(files, pair{h: h, p: filepath.Join(dir, e.Name())})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].h < files[j].h })

	maxH := int64(-1)
	if len(files) > 0 {
		maxH = files[len(files)-1].h
	}

	for _, f := range files {
		raw, err := os.ReadFile(f.p)
		if err != nil {
			return false, err
		}
		var b Block
		if err := decodePersistedBlock(raw, f.h, &b); err != nil {
			if f.h == maxH {
				if recoveryErr := handleReplayCorruption("post-snapshot block replay", f.p, fmt.Errorf("decode height %d: %w", f.h, err), true); recoveryErr == nil {
					continue
				} else {
					return false, recoveryErr
				}
			}
			return false, handleReplayCorruption("post-snapshot block replay", f.p, fmt.Errorf("decode height %d: %w", f.h, err), false)
		}
		// Use full validation
		if err := c.applyBlockLocked(b); err != nil {
			allowBestEffort := f.h == maxH
			return false, handleReplayCorruption("post-snapshot block replay", f.p, fmt.Errorf("apply height %d: %w", b.Height, err), allowBestEffort)
		}
	}

	return true, nil
}

// LoadSnapshotFromBytes applies a snapshot payload received over the mesh.
// It resets in-memory state to the snapshot base.
func (c *ProductionChain) LoadSnapshotFromBytes(raw []byte) (bool, error) {
	var snap ChainSnapshot
	if len(bytes.TrimSpace(raw)) == 0 {
		return false, fmt.Errorf("empty snapshot payload")
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		return false, err
	}
	accounts, err := validateSnapshotPayload(raw, snap)
	if err != nil {
		return false, err
	}

	// Reset chain state to validated snapshot
	c.height = snap.Height
	c.tip = snap.Hash
	c.accounts = accounts

	c.blocks = make(map[int64]Block)
	c.pending = make(map[int64]Block)
	c.resetMempoolLocked(MaxMempoolTxs)

	return true, nil
}

func validateSnapshotPayload(raw []byte, snap ChainSnapshot) (map[string]*Account, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("empty snapshot payload")
	}
	accounts := make(map[string]*Account, len(snap.Accounts))
	for k, v := range snap.Accounts {
		if v.Balance < 0 {
			return nil, fmt.Errorf("snapshot account %q has negative balance", k)
		}
		if v.Nonce < 0 {
			return nil, fmt.Errorf("snapshot account %q has negative nonce", k)
		}
		vv := v
		accounts[k] = &vv
	}
	if snap.AccountsHash == "" {
		return nil, fmt.Errorf("snapshot missing accounts_hash")
	}
	if computeAccountsHash(accounts) != snap.AccountsHash {
		return nil, fmt.Errorf("snapshot accounts_hash mismatch")
	}
	if snap.SnapshotHash == "" {
		return nil, fmt.Errorf("snapshot missing snapshot_hash")
	}
	if computeSnapshotHash(snap.Height, snap.Hash, snap.AccountsHash) != snap.SnapshotHash {
		return nil, fmt.Errorf("snapshot hash mismatch")
	}
	return accounts, nil
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
