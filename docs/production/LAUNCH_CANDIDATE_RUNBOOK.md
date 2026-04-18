# BlackChain Launch-Candidate Runbook

This runbook is Phase 9 scope only. It hardens launch review and release gating without changing runtime behavior, topology, or protocol.

## Production Readiness Checklist

Use this checklist against the exact launch candidate build and configuration set.

- Config loads cleanly with no missing required fields, no invalid addresses, no duplicate peers, no self-peers, and no `listen`/`http_listen` socket collision.
- Mesh transport mTLS is configured with valid cert, key, and CA files when mesh TLS is enabled.
- HTTP trust roots are present for internal HTTPS clients and `blackctl` where required.
- Startup succeeds with snapshot replay and block replay completing without incrementing replay failure counters.
- Corrupt persisted block or snapshot files do not silently pass startup. Default behavior must quarantine corrupt files and halt for operator action.
- `healthz` returns HTTP 200 on a healthy node.
- `readyz` returns HTTP 200 only after startup replay completes and the node marks `startup_ready=true`.
- `chain/status` exposes stable height, tip, and operator status fields.
- Bootstrap sync reaches parity from configured peers or explicit `peer_api` mappings without address derivation errors.
- Admin surface defaults are appropriate for the bind address and explicit overrides are documented in the launch config.
- Debug surface defaults are appropriate for the bind address and explicit overrides are documented in the launch config.
- Runtime peer mutation remains disabled unless there is an explicit operational reason to enable it.
- Rate limiting remains enabled for the production HTTP surface unless a deliberate exception is documented.
- Leader-only proposer paths remain restricted to the leader and admin-enabled nodes.
- Consensus API expectations remain aligned with [CONSENSUS_CORE_API.md](./CONSENSUS_CORE_API.md).
- Phase 6 transport and rate-limit expectations remain aligned with [PHASE6_LOCK.md](../PHASE6_LOCK.md).

## Staging Verification Checklist

Run these checks in staging before any launch candidate is promoted.

### Startup And Config Validation

- Start each node with the exact production-intended config shape.
- Confirm startup fails fast on malformed JSON, missing `node_id`, missing `http_listen`, invalid `listen`, invalid `host` or `port`, invalid peer addresses, duplicate peers, self-peers, and `listen == http_listen`.
- Confirm a bootstrap-only config is rejected as a mesh node config.
- Confirm operator status shows `startup_ready=true` only after replay completes.

### Replay And Corruption Behavior

- Start from a clean persisted state and confirm replay succeeds with `replay_failure_count=0`.
- Introduce a corrupt `snapshot.json` and confirm startup halts rather than continuing.
- Introduce a corrupt highest block file and confirm the file is quarantined under a `quarantine/` directory and startup halts by default.
- If best-effort corrupt recovery is ever exercised in staging, verify it only happens with `BLACKCHAIN_ALLOW_CORRUPT_RECOVERY=1` and is not part of the normal launch profile.
- Confirm `last_replay_error` is populated after a replay failure.

### TLS Trust Behavior

- For mesh TCP mTLS, verify both server and client cert validation against the configured CA.
- Confirm SAN coverage matches the actual dial targets used in staging.
- Confirm HTTP internal clients trust either the mesh CA or the shared HTTP CA as applicable.
- Confirm `blackctl` trust succeeds with the intended CA file and fails against an untrusted endpoint.
- Confirm `BLACKCHAIN_DEV_INSECURE_HTTP` is not required for the launch candidate path.

### Sync, Bootstrap, And API Address Resolution

- Start a node behind peers and confirm bootstrap sync reaches the best known height.
- Verify `peer_api` mappings work when explicitly configured.
- Verify implicit API derivation from mesh port works only where the deployment actually follows that port convention.
- Confirm staging catches any `peer api resolution failed` error before promotion.
- Confirm static bootstrap peers are sufficient for startup without topology expansion.

### Health, Readiness, And Status Endpoints

- `GET /healthz` returns HTTP 200 with `ok=true`.
- `GET /readyz` returns HTTP 503 before startup is ready and HTTP 200 after replay and initialization complete.
- `GET /chain/status` returns height, tip, finality fields, and operator status.
- Confirm operator status fields are sane for `startup_ready`, replay counters, sync counters, sync lag, reachable peer count, and rejected runtime peer mutations.

### Admin And Debug Surface Controls

- Confirm `/chain/propose` is admin-restricted and leader-restricted.
- Confirm `/chain/propose_broadcast` is admin-restricted and leader-restricted.
- Confirm `POST /peers` is admin-restricted and additionally requires `allow_runtime_peer_mutation=true`.
- Confirm `/debug/*` endpoints are debug-restricted.
- Confirm non-loopback HTTP listeners do not implicitly expose admin or debug surfaces unless explicitly enabled.
- Confirm loopback defaults are intentional for the staging environment being used.

## Rollback Guidance By Hardening Area

Rollback here means operational rollback to the last known-good launch candidate configuration and binary, not protocol or topology changes.

### Config Validation

- Revert to the last known-good config bundle.
- Remove the candidate config that introduced invalid bind, peer, or `peer_api` values.
- Restart only after config validation passes on disk.

### Replay And Corruption Handling

- Stop the affected node.
- Preserve the quarantined corrupt artifacts for analysis.
- Restore the previous known-good persisted state or snapshot set.
- Restart on the last known-good binary and config.

### TLS Trust And Certificates

- Revert to the last known-good cert, key, and CA set.
- Restore SAN coverage that matches actual dial targets.
- Revert any staging-only trust override that was introduced for candidate testing.

### Sync, Bootstrap, And API Resolution

- Revert to the last known-good `peers` and `peer_api` mapping set.
- Remove any candidate mapping that caused HTTP resolution or sync failures.
- Restart and confirm the node reaches parity before resuming promotion.

### Health And Readiness

- If `readyz` or status signals regress, roll back to the last candidate where startup replay and operator status were known-good.
- Do not promote a candidate that requires operators to ignore replay, sync, or readiness errors.

### Admin And Debug Surface Controls

- Revert to the last known-good admin/debug surface config values.
- Disable any accidental non-loopback admin or debug exposure before reattempting launch.
- Reset `allow_runtime_peer_mutation` to `false` unless explicitly required for the rollback scenario.

