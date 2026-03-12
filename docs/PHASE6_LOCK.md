# BlackChain — Phase 6 Lock (Security)

## Status
Phase 6 is **LOCKED GREEN**:
- (18) Encrypted Transport: **mTLS on mesh TCP** (cfg.tls.*) validated by openssl handshake and propagation.
- (19) Rate Limiting: **HTTP middleware** with per-IP token bucket, 429s on read spam, and **loopback write-exempt** for:
  - /chain/propose
  - /chain/propose_broadcast
  - /chain/tx
  - /chain/apply
  - /chain/snapshot/apply

## Proof Signals
- `openssl s_client` to mesh port succeeds with CA + client cert and returns `Verify return code: 0 (ok)`.
- `/chain/status` spam yields 429s.
- `POST /chain/propose_broadcast` is never blocked on loopback.
- Node2 restart -> resync -> tips match.

## Operational Notes
- Certs should include SAN for the dial target (IP or DNS).
- CA should be a real root (not concatenated self-signed leaf certs).
- Dev loopback uses `127.0.0.1` SAN.
