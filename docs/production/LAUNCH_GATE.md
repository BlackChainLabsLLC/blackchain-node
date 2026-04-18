# BlackChain Launch Gate

This is the final go/no-go gate for a Phase 9 launch candidate.

## Go Criteria

All items below must be true for the exact candidate binary and config set:

- The production readiness checklist in [LAUNCH_CANDIDATE_RUNBOOK.md](./LAUNCH_CANDIDATE_RUNBOOK.md) is fully complete.
- The staging verification checklist in [LAUNCH_CANDIDATE_RUNBOOK.md](./LAUNCH_CANDIDATE_RUNBOOK.md) is fully complete with no unresolved failures.
- No replay corruption, TLS trust, bootstrap sync, readiness, or surface-control regressions remain open.
- `/chain/propose` and `/chain/propose_broadcast` are admin-restricted and leader-restricted.
- `POST /peers` is admin-restricted and runtime-mutation restricted.
- `/debug/*` is debug-restricted.
- Release reviewers agree the candidate requires no deployment, topology, or protocol change beyond the already-approved launch plan.

## No-Go Triggers

Do not launch if any of the following is true:

- Startup depends on best-effort corruption recovery.
- TLS trust requires insecure dev overrides.
- `readyz` does not reliably reflect startup completion.
- Sync depends on undocumented `peer_api` assumptions or unresolved address derivation errors.
- Admin or debug surfaces are exposed beyond the intended bind and override policy.
- Rollback to the previous known-good launch candidate is not immediately available.

## Release Decision Record

Record the final decision with:

- candidate identifier
- config bundle identifier
- reviewer names
- date and time
- result: `GO` or `NO-GO`
- blocking issues, if any
