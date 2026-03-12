package mesh

// Phase 3.3 — Commit Threshold (single-node safe default).
// In single-node mode, threshold=1 means the local validator vote finalizes immediately.
// In multi-node mode, we can later wire this to a known validator set / quorum rules.
const defaultCommitThreshold = 1

func (c *ProductionChain) commitThresholdLocked() int {
	// TODO (Phase 3.x): compute from validator set / config.
	if defaultCommitThreshold < 1 {
		return 1
	}
	return defaultCommitThreshold
}

// tryFinalizeHeightLocked finalizes the block at height when enough votes exist.
// Caller must hold c.mu (write).
func (c *ProductionChain) tryFinalizeHeightLocked(height int64) bool {
	if height <= 0 {
		return false
	}

	b, ok := c.blocks[height]
	if !ok {
		return false
	}
	if b.IsFinalized {
		// keep tip consistent if this is our current height
		if height == c.height {
			c.tip = b.Hash
		}
		return true
	}

	need := c.commitThresholdLocked()

	vs := c.votes[height]
	if len(vs) == 0 {
		return false
	}

	// Count only votes that match THIS block hash at this height.
	got := 0
	for _, v := range vs {
		if v.Height == height && v.BlockHash == b.Hash {
			got++
		}
	}

	if got < need {
		return false
	}

	// Finalize
	b.IsFinalized = true
	c.blocks[height] = b
	_ = c.persistBlockLocked(b)

	// Advance tip when we finalize the current chain height.
	if height == c.height {
		c.tip = b.Hash
	}

	return true
}
