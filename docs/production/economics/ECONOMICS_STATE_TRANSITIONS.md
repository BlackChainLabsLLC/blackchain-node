# BlackChain Economics Layer (Production Spec)

This layer is applied strictly as deterministic state transitions AFTER a block is committed.

## Principles
- Deterministic: identical inputs yield identical state.
- Explicit: all balance changes are derived from block contents.
- Auditable: every transition has a recorded reason.

## Planned transitions (next implementation)
1) Transfer settlement (already present via tx apply).
2) Fee model (optional): deterministic fee schedule by tx type.
3) Mint/Burn hooks (optional): protocol-defined issuance schedules.
4) Invariants:
   - No negative balances
   - Total supply = previous supply + minted - burned

## Hook Interface (conceptual)
- OnBlockCommitted(block)
- OnTxApplied(tx)
- OnBlockApplied(block)
