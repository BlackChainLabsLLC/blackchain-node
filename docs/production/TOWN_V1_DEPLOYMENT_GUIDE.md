# BlackChain Town v1 Deployment Guide

This guide is the Town v1 operator handoff for the current BlackChain deployment shape.

## Deployment Shape

- binaries under `/usr/local/bin`
- configs under `/etc/blackchain/<node>/config.json`
- optional env files under `/etc/blackchain/<node>/blacknetd.env`
- data under `/var/lib/blackchain/<node>`
- systemd template at `/etc/systemd/system/blacknetd@.service`
- CA and node certs under `/etc/blackchain/tls`

## Operator Flow

1. Prepare a release manifest from [RELEASE_MANIFEST_TEMPLATE.yaml](/mnt/blacknet/projects/blackchain/deploy/release/RELEASE_MANIFEST_TEMPLATE.yaml).
2. Place binaries under `/usr/local/bin`.
3. Provision configs and env files with [provision_install_bundle.sh](/mnt/blacknet/projects/blackchain/scripts/provision_install_bundle.sh).
4. Place CA and node certs into `/etc/blackchain/tls`.
5. Install the systemd template and run `systemctl daemon-reload`.
6. Enable and start `blacknetd@bootstrap`, `blacknetd@node1`, `blacknetd@node2`, and `blacknetd@node3`.
7. Run [post_install_smoke.sh](/mnt/blacknet/projects/blackchain/scripts/post_install_smoke.sh).
8. Run [field_verify.sh](/mnt/blacknet/projects/blackchain/scripts/field_verify.sh).
9. Complete operator signoff in [TOWN_V1_ACCEPTANCE_SIGNOFF.md](/mnt/blacknet/projects/blackchain/docs/production/TOWN_V1_ACCEPTANCE_SIGNOFF.md).

## Supporting References

- [OPERATOR_DEPLOYMENT_CHECKLIST.md](/mnt/blacknet/projects/blackchain/docs/production/OPERATOR_DEPLOYMENT_CHECKLIST.md)
- [FIRST_RUN_PROVISIONING_CHECKLIST.md](/mnt/blacknet/projects/blackchain/docs/production/FIRST_RUN_PROVISIONING_CHECKLIST.md)
- [LAUNCH_CANDIDATE_RUNBOOK.md](/mnt/blacknet/projects/blackchain/docs/production/LAUNCH_CANDIDATE_RUNBOOK.md)
- [LAUNCH_GATE.md](/mnt/blacknet/projects/blackchain/docs/production/LAUNCH_GATE.md)

