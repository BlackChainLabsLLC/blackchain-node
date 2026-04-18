# BlackChain Website Publish Notes

## Scope

- static website only
- no backend form handler
- no topology or protocol changes

## Local Staging

- run `./website/publish_static_site.sh /tmp/blackchain-website-publish`
- confirm the staged directory contains:
  - `index.html`
  - `styles.css`
  - `app.js`
  - `pricing/index.html`
  - `host-site/index.html`
  - `operator/index.html`
  - `territory/index.html`
  - `territory/thanks/index.html`

## Publish Method

- copy the staged files to a static host that preserves directory routes
- publish the contents of the staged directory, not the repository root
- keep `/territory/thanks/` available as a static route

## Publish Guardrails

- keep CTA routes aligned to the current CTA map
- keep territory form fields aligned to the documented schema
- do not add backend capture in this sweep
