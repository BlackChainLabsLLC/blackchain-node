# BlackChain Finalization (Phase 3)

This module implements a minimal finalization layer:

- Validator identity: ed25519 keypair persisted to `<dataDir>/consensus/validator_key.json`
- Signed votes: per-block vote signed by validator private key
- Threshold: > 2/3 of validator set (equal power)
- Finalization: once threshold is reached for (height, blockHash), state is persisted to `<dataDir>/consensus/finality_state.json`
- Reorg protection: blocks at/below finalized height cannot be replaced

Wiring expectations:
- On block accepted/committed: `finalizer.ObserveBlock(height, hash)`
- On vote received (local or over mesh): verify signature, then `finalizer.AddVote(vote)`
- Before accepting any block: `finalizer.EnforceNoReorg(height, hash)`
