# Website Lead File Operations

## Working Files

- template: `website/data/territory_leads.template.ndjson`
- working lead file: `website/data/territory_leads.ndjson`

## Minimal Workflow

1. Initialize the working file with `./website/lead_file_ops.sh init`.
2. Save one captured lead record as a one-line JSON file.
3. Append it with `./website/lead_file_ops.sh append /path/to/lead.json`.
4. Export a copy with `./website/lead_file_ops.sh export /path/to/export.ndjson`.

## Notes

- keep one JSON object per line
- keep website field names unchanged
- treat `website/data/territory_leads.ndjson` as internal ops data, not published website content
