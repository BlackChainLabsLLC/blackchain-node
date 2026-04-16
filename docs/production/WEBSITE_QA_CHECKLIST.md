# BlackChain Website QA Checklist

## Page QA

- homepage renders with all documented sections
- pricing page renders with all three plan tiers
- host-site page renders with expected content blocks
- operator page renders with expected content blocks
- territory page renders with form and CTA

## Content QA

- copy matches the current source docs
- plan tier names match exactly
- hardware mapping matches exactly
- CTA labels match the CTA map

## Forms QA

- territory form required fields validate
- plan tier enum matches the documented values
- successful submit path is wired

## Tracking QA

- page view events fire on all current pages
- CTA events fire on all mapped CTAs
- territory form events fire on start, submit, success, and error

