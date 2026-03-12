# BlackChain Autodiscovery (Production Spec)

## Goal
Nodes discover peers automatically while supporting single-node operation.

## Constraints
- Works without central coordinator.
- Allows manual seed list override.
- Does not affect consensus correctness.

## Phases
1) Static seeds (current)
2) Local LAN discovery (mDNS) + optional
3) DHT-style discovery (later)
