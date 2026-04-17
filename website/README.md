# BlackChain Static Website Builder Notes

- serve the static site from the `website/` directory only
- preserve the current route set:
  - `/`
  - `/pricing/`
  - `/host-site/`
  - `/operator/`
  - `/territory/`
  - `/territory/thanks/`
  - `/leads/`
  - `/founder/`
- keep CTA targets and form field names aligned to the production docs before publishing
- local preview:
  - `python3 -m http.server 8123 --directory website`
- local publish staging:
  - `./website/publish_static_site.sh /tmp/blackchain-website-publish`
- publish by copying the contents of `website/` to any static host that preserves directory routes
- do not add backend form handling in this sweep

## Deployment And Workflow Pack

- [WEBSITE_PUBLISH_NOTES.md](/mnt/blacknet/projects/blackchain/website/docs/WEBSITE_PUBLISH_NOTES.md)
- [TERRITORY_FORM_CAPTURE_WORKFLOW.md](/mnt/blacknet/projects/blackchain/website/docs/TERRITORY_FORM_CAPTURE_WORKFLOW.md)
- [LEAD_INTAKE_STORAGE_AND_EXPORT.md](/mnt/blacknet/projects/blackchain/website/docs/LEAD_INTAKE_STORAGE_AND_EXPORT.md)
- [WEBSITE_RELEASE_CHECKLIST.md](/mnt/blacknet/projects/blackchain/website/docs/WEBSITE_RELEASE_CHECKLIST.md)
- [WEBSITE_PUBLISH_ROLLBACK.md](/mnt/blacknet/projects/blackchain/website/docs/WEBSITE_PUBLISH_ROLLBACK.md)
- [STATIC_HOSTING_RECOMMENDATION.md](/mnt/blacknet/projects/blackchain/website/docs/STATIC_HOSTING_RECOMMENDATION.md)
- [WEBSITE_LEAD_HANDOFF_CHECKLIST.md](/mnt/blacknet/projects/blackchain/website/docs/WEBSITE_LEAD_HANDOFF_CHECKLIST.md)
- [TERRITORY_INTAKE_REVIEW_TEMPLATE.md](/mnt/blacknet/projects/blackchain/website/docs/TERRITORY_INTAKE_REVIEW_TEMPLATE.md)
- [WEBSITE_LEAD_FIRST_RESPONSE_WORKFLOW.md](/mnt/blacknet/projects/blackchain/website/docs/WEBSITE_LEAD_FIRST_RESPONSE_WORKFLOW.md)
- [WEBSITE_LEAD_STATUS_TRACKER_TEMPLATE.md](/mnt/blacknet/projects/blackchain/website/docs/WEBSITE_LEAD_STATUS_TRACKER_TEMPLATE.md)
- [WEBSITE_PUBLISH_TO_LAUNCH_HANDOFF.md](/mnt/blacknet/projects/blackchain/website/docs/WEBSITE_PUBLISH_TO_LAUNCH_HANDOFF.md)
- [WEBSITE_STATIC_LEAD_PRIVACY_NOTE.md](/mnt/blacknet/projects/blackchain/website/docs/WEBSITE_STATIC_LEAD_PRIVACY_NOTE.md)
- [WEBSITE_LEAD_FIRST_RESPONSE_TEMPLATES.md](/mnt/blacknet/projects/blackchain/website/docs/WEBSITE_LEAD_FIRST_RESPONSE_TEMPLATES.md)
- [WEBSITE_LEAD_QUALIFICATION_CHECKLIST.md](/mnt/blacknet/projects/blackchain/website/docs/WEBSITE_LEAD_QUALIFICATION_CHECKLIST.md)
- [WEBSITE_TO_TERRITORY_PIPELINE_HANDOFF.md](/mnt/blacknet/projects/blackchain/website/docs/WEBSITE_TO_TERRITORY_PIPELINE_HANDOFF.md)
- [FOUNDER_DAILY_DASHBOARD.md](/mnt/blacknet/projects/blackchain/website/docs/FOUNDER_DAILY_DASHBOARD.md)
- [FOUNDER_COMBINED_SUMMARY_SHEET.md](/mnt/blacknet/projects/blackchain/website/docs/FOUNDER_COMBINED_SUMMARY_SHEET.md)
- [FOUNDER_TODAYS_PRIORITIES.md](/mnt/blacknet/projects/blackchain/website/docs/FOUNDER_TODAYS_PRIORITIES.md)
- [FOUNDER_REVIEW_RHYTHM_GUIDE.md](/mnt/blacknet/projects/blackchain/website/docs/FOUNDER_REVIEW_RHYTHM_GUIDE.md)
- [FOUNDER_DECISION_QUEUE_TEMPLATE.md](/mnt/blacknet/projects/blackchain/website/docs/FOUNDER_DECISION_QUEUE_TEMPLATE.md)
- [FOUNDER_FIRST_LOOK_EVERY_DAY.md](/mnt/blacknet/projects/blackchain/website/docs/FOUNDER_FIRST_LOOK_EVERY_DAY.md)
- [Lead Workflow Surface](/mnt/blacknet/projects/blackchain/website/leads/index.html)
- [Founder Command Center](/mnt/blacknet/projects/blackchain/website/founder/index.html)
