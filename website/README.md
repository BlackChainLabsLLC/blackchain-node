# BlackChain Static Website Builder Notes

- serve the static site from the `website/` directory only
- preserve the current route set:
  - `/`
  - `/pricing/`
  - `/host-site/`
  - `/operator/`
  - `/territory/`
  - `/territory/thanks/`
- keep CTA targets and form field names aligned to the production docs before publishing
- local preview:
  - `python3 -m http.server 8123 --directory website`
- publish by copying the contents of `website/` to any static host that preserves directory routes
- do not add backend form handling in this sweep
