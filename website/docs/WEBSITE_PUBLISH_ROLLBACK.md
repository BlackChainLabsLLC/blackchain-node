# Website Publish Rollback

## Goal

- restore the last known-good static website publish without changing topology or runtime behavior

## Rollback Method

1. Identify the last known-good staged website artifact or published directory snapshot.
2. Replace the current published static files with that prior artifact.
3. Re-check the six static routes:
   - `/`
   - `/pricing/`
   - `/host-site/`
   - `/operator/`
   - `/territory/`
   - `/territory/thanks/`
4. Re-check the territory submit redirect path.

## Notes

- this rollback is content and static-asset rollback only
- do not change protocol, deployment topology, or node runtime settings
- if a publish target supports versioned deploys, prefer switching back to the last known-good website version
