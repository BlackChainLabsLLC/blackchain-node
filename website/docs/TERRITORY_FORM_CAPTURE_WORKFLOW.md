# Territory Form Capture Workflow

## Current Reality

- the website is static
- the territory form validates and routes to a static thank-you page
- no server-side submission capture is added in this sweep

## Capture Workflow

1. Use the territory form during live review, founder outreach, or operator calls.
2. After submit, confirm the browser reaches `/territory/thanks/`.
3. Open browser storage for the current session and copy `blackchainLastTerritorySubmission`.
4. Paste the captured record into the lead intake store.
5. Add any follow-up context from the conversation in the `notes` field of the intake record, not by changing the website payload.

## Minimum Capture Fields

- name
- organization
- email
- phone
- territory_or_town
- host_site_interest
- operator_interest
- preferred_plan_tier
- notes

## Handling Notes

- keep field names unchanged from the website form
- preserve free text as entered
- if no browser storage is available, record the lead manually using the same field names
