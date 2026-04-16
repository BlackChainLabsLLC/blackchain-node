# BlackChain Website Form Field Schema

## Territory Interest Form

| Field | Type | Required |
| --- | --- | --- |
| name | text | yes |
| organization | text | yes |
| email | email | yes |
| phone | text | no |
| territory_or_town | text | yes |
| host_site_interest | boolean | yes |
| operator_interest | boolean | yes |
| preferred_plan_tier | enum | no |
| notes | textarea | no |

## Preferred Plan Tier Enum

- Standard
- Premium
- Family/Business

