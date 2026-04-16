# Website To Territory Pipeline Handoff

## Goal

- move qualified website leads into the territory pipeline tracker without changing the website flow

## Source Reference

- use [TERRITORY_PIPELINE_TRACKER_TEMPLATE.md](/mnt/blacknet/projects/blackchain/docs/production/TERRITORY_PIPELINE_TRACKER_TEMPLATE.md)

## Handoff Mapping

- `territory_or_town` -> `Territory`
- `status=qualified` -> `Stage=lead`
- `host_site_interest=yes` -> `Host-Site Identified=YES`
- `host_site_interest=no` -> `Host-Site Identified=NO`
- `operator_interest=yes` -> `Operator Identified=YES`
- `operator_interest=no` -> `Operator Identified=NO`
- `preferred_plan_tier` -> `Plan Tier Focus`
- assigned next action -> `Next Step`
- lead owner -> `Owner`
- review notes -> `Notes`

## Handoff Steps

1. Confirm the website lead is marked `qualified`.
2. Open the territory pipeline tracker.
3. Copy the mapped fields into a new tracker row.
4. Set the stage to `lead`.
5. Keep the website lead record and pipeline row aligned after each follow-up.

## Notes

- do not overwrite the original website lead record
- use the tracker for territory progression after qualification
