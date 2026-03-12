# BlackChain Trust Scoring (Production Spec)

## Goal
Maintain a local reputation/trust score for peers to improve routing and reduce spam.

## Inputs
- Reachability (connect success rate)
- Liveness (heartbeat quality)
- Message validity (signature checks, schema compliance)
- Rate limits exceeded events

## Outputs
- A score in [0..100]
- Enforcement actions:
  - degrade relay priority
  - temporary quarantine
  - ban (time-boxed)
