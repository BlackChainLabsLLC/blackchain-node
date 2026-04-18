# BlackChain Staging Execution Worksheet

Use this worksheet during launch-candidate staging validation. Complete it against the exact candidate binary and config bundle under review.

References:

- [LAUNCH_CANDIDATE_RUNBOOK.md](./LAUNCH_CANDIDATE_RUNBOOK.md)
- [LAUNCH_GATE.md](./LAUNCH_GATE.md)

## Candidate Record

- candidate commit:
- candidate tag or build identifier:
- config bundle identifier:
- test date:
- environment:
- operators:
- reviewers:

## Node-By-Node Startup Results

| Node | Config file or bundle ref | Started cleanly | `startup_ready` | Replay failures | Height after startup | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| node1 |  | PASS / FAIL | TRUE / FALSE |  |  |  |
| node2 |  | PASS / FAIL | TRUE / FALSE |  |  |  |
| node3 |  | PASS / FAIL | TRUE / FALSE |  |  |  |
| node4 |  | PASS / FAIL | TRUE / FALSE |  |  |  |
| node5 |  | PASS / FAIL | TRUE / FALSE |  |  |  |

## Startup And Config Validation

| Check | Result | Evidence | Notes |
| --- | --- | --- | --- |
| Candidate config shape matches intended production layout | PASS / FAIL |  |  |
| Missing required fields are rejected | PASS / FAIL |  |  |
| Invalid `listen` or `http_listen` is rejected | PASS / FAIL |  |  |
| Duplicate peers are rejected | PASS / FAIL |  |  |
| Self-peer entries are rejected | PASS / FAIL |  |  |
| `listen` and `http_listen` collision is rejected | PASS / FAIL |  |  |
| Bootstrap-only config is rejected as a node config | PASS / FAIL |  |  |

## Replay And Corruption Checks

| Check | Result | Evidence | Notes |
| --- | --- | --- | --- |
| Clean replay succeeds with `replay_failure_count=0` | PASS / FAIL |  |  |
| Corrupt `snapshot.json` halts startup | PASS / FAIL |  |  |
| Corrupt highest block file is quarantined | PASS / FAIL |  |  |
| Startup remains halted by default after replay corruption | PASS / FAIL |  |  |
| `last_replay_error` is populated after failure | PASS / FAIL |  |  |
| Best-effort corrupt recovery is not required for candidate validation | PASS / FAIL |  |  |

## TLS Trust Checks

| Check | Result | Evidence | Notes |
| --- | --- | --- | --- |
| Mesh TCP mTLS handshake succeeds with intended CA | PASS / FAIL |  |  |
| Server cert SAN matches actual dial targets | PASS / FAIL |  |  |
| Client cert trust is enforced where expected | PASS / FAIL |  |  |
| Internal HTTPS trust succeeds with intended CA roots | PASS / FAIL |  |  |
| `blackctl` trust succeeds with intended CA file | PASS / FAIL |  |  |
| Untrusted endpoint fails as expected | PASS / FAIL |  |  |
| No insecure dev HTTP override is required | PASS / FAIL |  |  |

## Sync, Bootstrap, And API Resolution Checks

| Check | Result | Evidence | Notes |
| --- | --- | --- | --- |
| Bootstrap sync reaches expected best height | PASS / FAIL |  |  |
| Explicit `peer_api` mapping works | PASS / FAIL |  |  |
| Implicit API derivation works where expected | PASS / FAIL |  |  |
| No unresolved peer API resolution errors remain | PASS / FAIL |  |  |
| Static bootstrap peers are sufficient for staging startup | PASS / FAIL |  |  |
| No topology expansion was required during candidate test | PASS / FAIL |  |  |

## Health, Readiness, And Status Checks

| Check | Result | Evidence | Notes |
| --- | --- | --- | --- |
| `GET /healthz` returns HTTP 200 with `ok=true` | PASS / FAIL |  |  |
| `GET /readyz` returns HTTP 503 before startup completes | PASS / FAIL |  |  |
| `GET /readyz` returns HTTP 200 after startup completes | PASS / FAIL |  |  |
| `GET /chain/status` returns expected height and tip | PASS / FAIL |  |  |
| Operator status fields are sane | PASS / FAIL |  |  |
| Sync lag is acceptable for the staging test window | PASS / FAIL |  |  |

## Admin And Debug Surface Checks

| Check | Result | Evidence | Notes |
| --- | --- | --- | --- |
| `/chain/propose` is admin-restricted | PASS / FAIL |  |  |
| `/chain/propose` is leader-restricted | PASS / FAIL |  |  |
| `/chain/propose_broadcast` is admin-restricted | PASS / FAIL |  |  |
| `/chain/propose_broadcast` is leader-restricted | PASS / FAIL |  |  |
| `POST /peers` is admin-restricted | PASS / FAIL |  |  |
| `POST /peers` also requires `allow_runtime_peer_mutation=true` | PASS / FAIL |  |  |
| `/debug/*` is debug-restricted | PASS / FAIL |  |  |
| Non-loopback listeners do not implicitly expose admin or debug surfaces | PASS / FAIL |  |  |

## Rollback Readiness

| Item | Result | Evidence | Notes |
| --- | --- | --- | --- |
| Previous known-good candidate binary is available | PASS / FAIL |  |  |
| Previous known-good config bundle is available | PASS / FAIL |  |  |
| Previous known-good cert and CA set is available | PASS / FAIL |  |  |
| Previous known-good persisted state or snapshot set is available | PASS / FAIL |  |  |
| Operators can execute rollback without topology or protocol change | PASS / FAIL |  |  |
| Rollback owner is assigned | PASS / FAIL |  |  |

## Blocking Issues

| ID | Severity | Summary | Owner | Status |
| --- | --- | --- | --- | --- |
| 1 |  |  |  |  |
| 2 |  |  |  |  |
| 3 |  |  |  |  |

## Final Decision

- go / no-go decision:
- decision timestamp:
- approvers:
- blocking issues remaining:
- follow-up actions:
- rollback plan reference:

