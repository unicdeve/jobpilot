# Feature Index

Feature specifications define behavior and acceptance criteria.
See [current state](../00_product/current_state.md) for implementation limitations and verification status.

## Implemented workflows

| Area | Specifications |
| --- | --- |
| Setup and candidate context | [Onboarding](01_onboarding.md), [Profile](02_candidate_profile.md), [Resumes](03_resume_management.md), [About you](16_about_you.md) |
| Discovery and analysis | [Discovery](04_job_discovery.md), [Matching](05_job_matching.md), [Search templates](14_search_templates.md), [Company watchlists](17_company_watchlists.md), [Working hours](18_time_zones.md) |
| Application preparation | [Cover letters](06_cover_letters.md), [Answers](07_application_answers.md), [Applications](08_applications.md), [Review mode](09_review_mode.md), [Draft checks](19_draft_checks.md) |
| Background work and activity | [Scouts](10_scouts.md), [Notifications](11_notifications.md), [Activity](12_agent_activity.md) |
| Configuration and data | [Settings](13_settings.md), [Data deletion](15_data_deletion.md), [Prompt customization](20_prompt_customization.md) |

The [CLI reference](../CLI.md) and [desktop architecture](../01_architecture/02_desktop.md) cover the interfaces.
[Browser preparation](../01_architecture/03_browser_worker.md) and Review mode are implemented; the user always submits applications themselves.

## Remaining and proposed work

- Settings categories beyond those implemented, including retention policies and browser configuration.
- Email notification delivery; desktop notifications are implemented.
- Additional sources, including keyed adapters.
- External connector/tool SDKs and the [plugin runtime](../05_plugins/00_plugin_architecture.md).
- In-app updating; [packaging](../08_development/packaging.md) uses manual updates.

Unattended browser preparation remains deferred.
Automatic submission is permanently excluded by [ADR-0014](../09_decisions/0014-scouts-and-no-unattended-submission.md).
