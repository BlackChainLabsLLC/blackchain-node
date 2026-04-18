# BlackChain Launch FAQ For Operators And Support

## What paths should operators expect on the host?

- binaries in `/usr/local/bin`
- configs in `/etc/blackchain`
- data in `/var/lib/blackchain`
- systemd template at `/etc/systemd/system/blacknetd@.service`

## What units should be running in the current deployment shape?

- `blacknetd@bootstrap`
- `blacknetd@node1`
- `blacknetd@node2`
- `blacknetd@node3`

## What should operators verify first after launch?

- service status
- listeners
- HTTPS `healthz`, `readyz`, and `chain/status`
- `blackctl` HTTPS trust
- finality snapshot visibility

## Which hardware maps to which launch plan?

- Standard -> Pi Zero
- Premium -> Pi 3
- Family/Business -> Pi 4

## When should support escalate?

- use the Town v1 incident escalation matrix when outages, readiness loss, replay failures, or sustained sync issues are observed

## What should support capture in every case?

- customer or site
- affected node or nodes
- severity
- evidence
- immediate action
- next update due

