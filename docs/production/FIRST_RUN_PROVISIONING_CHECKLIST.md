# BlackChain First-Run Provisioning Checklist

This checklist covers first deployment for the current BlackChain shape using the install bundle and productization scripts.

Related assets:

- install templates under [deploy/install](/mnt/blacknet/projects/blackchain/deploy/install)
- bundle helper: [provision_install_bundle.sh](/mnt/blacknet/projects/blackchain/scripts/provision_install_bundle.sh)
- post-install smoke: [post_install_smoke.sh](/mnt/blacknet/projects/blackchain/scripts/post_install_smoke.sh)
- field verification: [field_verify.sh](/mnt/blacknet/projects/blackchain/scripts/field_verify.sh)

## Binary Placement

- Install `blacknetd` to `/usr/local/bin/blacknetd`
- Install `blackctl` to `/usr/local/bin/blackctl`
- Install `signtx` to `/usr/local/bin/signtx` if used

## Config Placement

- Copy canonical templates to `/etc/blackchain/bootstrap/config.json`
- Copy canonical templates to `/etc/blackchain/node1/config.json`
- Copy canonical templates to `/etc/blackchain/node2/config.json`
- Copy canonical templates to `/etc/blackchain/node3/config.json`
- Copy optional env files to `/etc/blackchain/<node>/blacknetd.env` if needed

## Cert And CA Placement

- Install CA to `/etc/blackchain/tls/ca.pem`
- Install node cert and key to:
  - `/etc/blackchain/tls/bootstrap/`
  - `/etc/blackchain/tls/node1/`
  - `/etc/blackchain/tls/node2/`
  - `/etc/blackchain/tls/node3/`
- Confirm cert ownership and permissions match operational policy

## Service Enable And Start

- Install `/etc/systemd/system/blacknetd@.service`
- Run `sudo systemctl daemon-reload`
- Enable and start:
  - `blacknetd@bootstrap`
  - `blacknetd@node1`
  - `blacknetd@node2`
  - `blacknetd@node3`

## Initial Field Verification

- Run `scripts/post_install_smoke.sh`
- Run `scripts/field_verify.sh`
- Confirm:
  - services are active
  - expected listeners are present
  - HTTPS endpoints respond successfully
  - `blackctl` HTTPS trust succeeds
  - finality snapshots are readable

