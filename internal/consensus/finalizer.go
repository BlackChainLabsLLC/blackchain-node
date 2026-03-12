package consensus

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FinalityState struct {
	FinalizedHeight uint64 `json:"finalized_height"`
	FinalizedHash   string `json:"finalized_hash"` // hex
	UpdatedAt       string `json:"updated_at"`     // RFC3339
}

type Finalizer struct {
	mu sync.Mutex

	dataDir string

	// Validator set (MVP): equal power, N validators
	validators map[string]bool // validatorID -> true
	totalPower uint64          // = len(validators)
	threshold  uint64          // ceil(2/3*totalPower)

	// Observed blocks
	blocks map[uint64]map[string]bool // height -> blockHash -> seen

	// Votes: height -> blockHash -> validatorID -> true
	votes map[uint64]map[string]map[string]bool

	state FinalityState
}

func NewFinalizer(dataDir string, validatorIDs []string) (*Finalizer, error) {
	if dataDir == "" {
		return nil, errors.New("dataDir is empty")
	}
	vset := make(map[string]bool, len(validatorIDs))
	for _, id := range validatorIDs {
		if id != "" {
			vset[id] = true
		}
	}
	if len(vset) == 0 {
		return nil, errors.New("validator set empty")
	}
	f := &Finalizer{
		dataDir:    dataDir,
		validators: vset,
		totalPower: uint64(len(vset)),
		blocks:     make(map[uint64]map[string]bool),
		votes:      make(map[uint64]map[string]map[string]bool),
		state:      FinalityState{},
	}
	f.threshold = (2*f.totalPower)/3 + 1 // > 2/3
	_ = f.load()
	return f, nil
}

func (f *Finalizer) State() FinalityState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

func (f *Finalizer) ObserveBlock(height uint64, blockHashHex string) {
	if height == 0 || blockHashHex == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.blocks[height] == nil {
		f.blocks[height] = make(map[string]bool)
	}
	f.blocks[height][blockHashHex] = true
}

func (f *Finalizer) AddVote(v Vote) (finalized bool, newState FinalityState, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Basic validator-set gate (MVP)
	if !f.validators[v.ValidatorID] {
		return false, f.state, fmt.Errorf("vote from non-validator: %s", v.ValidatorID)
	}

	if f.votes[v.Height] == nil {
		f.votes[v.Height] = make(map[string]map[string]bool)
	}
	if f.votes[v.Height][v.BlockHash] == nil {
		f.votes[v.Height][v.BlockHash] = make(map[string]bool)
	}
	f.votes[v.Height][v.BlockHash][v.ValidatorID] = true

	power := uint64(len(f.votes[v.Height][v.BlockHash]))
	if power < f.threshold {
		return false, f.state, nil
	}

	// Finalize monotonically (no skipping checks here; keep MVP)
	if v.Height > f.state.FinalizedHeight {
		f.state.FinalizedHeight = v.Height
		f.state.FinalizedHash = v.BlockHash
		f.state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = f.persist()
		return true, f.state, nil
	}
	return false, f.state, nil
}

func (f *Finalizer) EnforceNoReorg(height uint64, blockHashHex string) error {
	// Reject any attempt to accept a different block at or below finalized height
	if height == 0 {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state.FinalizedHeight == 0 {
		return nil
	}
	if height < f.state.FinalizedHeight {
		return fmt.Errorf("cannot modify history below finalized height=%d", f.state.FinalizedHeight)
	}
	if height == f.state.FinalizedHeight && f.state.FinalizedHash != "" && blockHashHex != "" && blockHashHex != f.state.FinalizedHash {
		return fmt.Errorf("cannot reorg finalized height=%d (expected %s got %s)", height, f.state.FinalizedHash, blockHashHex)
	}
	return nil
}

func (f *Finalizer) load() error {
	path := filepath.Join(f.dataDir, "consensus", "finality_state.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil // ok if missing
	}
	var st FinalityState
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	f.state = st
	return nil
}

func (f *Finalizer) persist() error {
	dir := filepath.Join(f.dataDir, "consensus")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "finality_state.json")
	b, err := json.MarshalIndent(&f.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
