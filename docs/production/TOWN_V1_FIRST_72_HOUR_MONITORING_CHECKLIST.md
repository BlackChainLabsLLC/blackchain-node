# BlackChain Town v1 First 72-Hour Monitoring Checklist

Use this checklist during the first 72 hours after Town v1 go-live.

## Monitoring Cadence

- 0-6 hours: check every 30 minutes
- 6-24 hours: check every 2 hours
- 24-72 hours: check every 8 hours

## Service And Listener Checks

- all target units remain `active`
- expected listeners remain present on `6069`, `6060`, `6061`, `6062`, `7071`, `7072`, `7073`, `7074`
- no unexpected restart loop is observed

## HTTPS And Trust Checks

- `healthz` succeeds over HTTPS on all current nodes
- `readyz` remains successful on all current nodes
- `chain/status` remains readable on all current nodes
- `blackctl` HTTPS trust remains functional against node1

## Chain And Sync Checks

- node heights remain aligned or within expected sync lag
- finality snapshots remain readable
- `startup_ready` remains true
- replay failure counters remain zero unless an incident is open

## Surface Control Checks

- admin and debug surfaces remain in the expected exposure state
- unexpected runtime peer mutation rejection spikes are investigated

## Artifact Capture

- save each monitoring pass as a run artifact
- record any deviation and operator action taken

