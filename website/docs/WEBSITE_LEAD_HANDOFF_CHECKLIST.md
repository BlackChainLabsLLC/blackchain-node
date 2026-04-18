# Website Lead Handoff Checklist

## Goal

- move a captured website territory lead into ops review without changing the website runtime

## Handoff Steps

1. Confirm the lead came from the territory website flow.
2. Confirm the submit path reached `/territory/thanks/`.
3. Copy the stored submission payload using the documented field names.
4. Add the lead to the intake store or tracker.
5. Record `captured_at` in UTC.
6. Record the source as `website_territory_form`.
7. Flag whether host-site interest is `yes` or `no`.
8. Flag whether operator interest is `yes` or `no`.
9. Assign an owner for ops review.
10. Set the first review status.

## Minimum Handoff Record

- captured_at
- source
- name
- organization
- email
- phone
- territory_or_town
- host_site_interest
- operator_interest
- preferred_plan_tier
- notes
- owner
- status

## Notes

- do not rename website fields during handoff
- preserve free text before ops adds review notes
