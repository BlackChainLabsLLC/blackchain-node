# BlackChain Operator Deployment Checklist

This checklist is aligned to the current deployment shape and the repo scripts:

- manifest template: [RELEASE_MANIFEST_TEMPLATE.yaml](/mnt/blacknet/projects/blackchain/deploy/release/RELEASE_MANIFEST_TEMPLATE.yaml)
- promotion script: [release_promote.sh](/mnt/blacknet/projects/blackchain/scripts/release_promote.sh)
- rollback script: [release_rollback.sh](/mnt/blacknet/projects/blackchain/scripts/release_rollback.sh)
- field verification script: [field_verify.sh](/mnt/blacknet/projects/blackchain/scripts/field_verify.sh)

## Pre-Promotion

- Build the candidate binaries intended for `/usr/local/bin`.
- Fill out a release manifest from the template with commit, binary hashes, config refs, CA refs, service refs, and staging run refs.
- Confirm the target units match the current runtime topology.
- Confirm the promotion operator has `sudo` access for `/usr/local/bin`, `/etc/systemd/system`, and `systemctl`.

## Promotion

- Run `scripts/release_promote.sh`.
- Record the emitted `state_dir` for rollback.
- Confirm the expected units restarted successfully.

## Field Verification

- Run `scripts/field_verify.sh`.
- Confirm:
  - services are active
  - expected listeners are present
  - HTTPS `healthz`, `readyz`, and `chain/status` succeed
  - `blackctl` succeeds over HTTPS trust
  - finality snapshots are readable

## Rollback

- If promotion fails, run `scripts/release_rollback.sh` with the recorded `STATE_DIR`.
- Re-run `scripts/field_verify.sh` after rollback.
- Record the rollback outcome in the release manifest or run artifact set.

## Current Deployment Assumptions

- binaries under `/usr/local/bin`
- systemd template at `/etc/systemd/system/blacknetd@.service`
- node configs under `/etc/blackchain/<node>/config.json`
- node data under `/var/lib/blackchain/<node>`
- CA trust rooted at `/etc/blackchain/tls/ca.pem` or repo-local `tls/ca.pem`

