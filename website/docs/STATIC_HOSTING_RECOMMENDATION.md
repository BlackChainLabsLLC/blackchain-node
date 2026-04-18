# Static Hosting Recommendation

## Recommended Fit

- use a plain static host that preserves directory routes and can publish a folder as-is

## Why This Fits The Current Sweep

- no build step is required
- no server-side rendering is required
- no backend form endpoint is required
- the route set is small and fixed

## Minimum Host Requirements

- serve `index.html` files from route directories
- preserve `/territory/thanks/`
- allow replacing the full static artifact for publish rollback

## Recommendation

- use any static host that can publish the staged output of `./website/publish_static_site.sh`
- prefer a host with simple directory-based deploys and easy revert to a prior publish
