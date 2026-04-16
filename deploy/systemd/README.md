# Systemd Assets

These files are repo-only hardening assets for future rollout. They do not
change the current installers or any live host behavior.

Files:
- `blacknetd@.service`: hardened template unit for node instances
- `blacknetd.env.example`: repo-only example environment file for rollout notes

Notes:
- `ExecStart` uses the explicit `mesh` subcommand and absolute config/data paths.
- `systemd-analyze verify` rejected the earlier unit because `WorkingDirectory=`
  must be an absolute path at verification/load time, and
  `${BLACKCHAIN_ROOT}` is not accepted there as a literal absolute path.
- `WorkingDirectory` remains explicit because current bootstrap loading uses the
  process working directory to resolve `config/bootstrap.json`.
- `Restart=on-failure`, start limits, and stop timeouts are set for cleaner
  restart behavior and orderly shutdown.
- Sandboxing is enabled with `NoNewPrivileges`, `PrivateTmp`,
  `ProtectSystem=strict`, `ProtectHome=true`, and narrowed `ReadWritePaths`.
- The template expects:
  - config under `/etc/blackchain/<node_id>/config.json`
  - data under `/var/lib/blackchain/<node_id>/`
  - the repo checkout or release root at `/opt/blackchain/current`
  - an optional env file at `/etc/blackchain/<node_id>/blacknetd.env`

Example rollout:

```ini
sudo install -m 644 deploy/systemd/blacknetd@.service /etc/systemd/system/blacknetd@.service
sudo install -m 644 deploy/systemd/blacknetd.env.example /etc/blackchain/node1/blacknetd.env
sudo systemctl daemon-reload
sudo systemctl enable --now blacknetd@node1
```

The example above is documentation only. This repository change does not apply
or modify any live systemd units.
