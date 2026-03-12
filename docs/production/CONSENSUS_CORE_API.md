# BlackChain Consensus Core API Contract (Production)

This contract defines the production API surface for consensus finality. This document is normative.

## Invariants
- Block hashes are deterministic over (height, prev_hash, tx list, producer pub, sig).
- Finality is defined by a valid set of approvals over `APPROVE|<block_hash>`.
- A block is considered committed only after `commit` succeeds.
- Followers converge by importing commits (`commit/import`) and applying the block.

## Endpoints (Leader & Followers)
### GET /chain/status
Returns:
- `height` (uint)
- `hash` (string) current tip

### POST /chain/tx
Accepts a transaction for mempool inclusion.

### POST /chain/propose
Leader proposes next block.
Returns full Block object.

### POST /chain/approve
Body: `{ "hash": "<block_hash>" }`
Returns: `{ ok: true, approval: { node_id, pub, sig } }`

### POST /chain/commit
Body: `{ "block": <Block>, "approvals": [<Approval>...] }`
- Must validate leader sig on block.
- Must validate approvals threshold for finality.
- On leader, applies block locally.

### POST /chain/commit/import?leader=<leader_url>
Body: `{ "block": <Block>, "approvals": [<Approval>...] }`
- Followers import committed block and apply it.
- Join-late catch-up is allowed prior to import.

## Operational Requirement
Production operators MUST:
- Collect approvals from multiple nodes.
- Commit once on leader.
- Propagate commit/import to followers.
