# Website Release Checklist

## Content

- homepage copy matches the current source doc
- pricing tiers and hardware mapping match exactly
- host-site and operator CTAs match the CTA map
- territory form labels and field names match the documented schema

## Routes

- `/` loads
- `/pricing/` loads
- `/host-site/` loads
- `/operator/` loads
- `/territory/` loads
- `/territory/thanks/` loads

## Form And Tracking

- territory form required fields validate
- successful submit redirects to `/territory/thanks/`
- page view and CTA event names remain aligned to the current analytics plan
- territory form event names remain:
  - `territory_form_view`
  - `territory_form_start`
  - `territory_form_submit`
  - `territory_form_submit_success`
  - `territory_form_submit_error`

## Publish

- static files staged with `./website/publish_static_site.sh`
- staged output contains only website assets and route directories
- rollback note is available before publish
