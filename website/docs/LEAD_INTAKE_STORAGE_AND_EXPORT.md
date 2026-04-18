# Lead Intake Storage And Export

## Minimal Storage Path

- store captured territory submissions in a flat file at `website/data/territory_leads.ndjson`
- keep one JSON object per line
- keep the website payload fields unchanged

## Record Shape

```json
{"captured_at":"2026-04-17T00:00:00Z","source":"website_territory_form","name":"","organization":"","email":"","phone":"","territory_or_town":"","host_site_interest":"","operator_interest":"","preferred_plan_tier":"","notes":""}
```

## Export Path

- export by copying `website/data/territory_leads.ndjson`
- optional CSV export can be created from the NDJSON file outside the website runtime
- do not make CSV export a runtime dependency for the static site

## Operational Notes

- create `website/data/` only when you are actually storing captured leads
- keep this file out of the published static artifact
- treat the file as internal operational data, not website content
