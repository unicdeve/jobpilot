# Application Modes

## Assist

The agent discovers and analyzes jobs and generates application content.
It does not interact with external application forms.

## Review - Default

The agent prepares and fills an application form, then leaves the browser window open for the user to review and submit themselves.
Approval in JobPilot records the review decision; it never sends an application to an employer.

## Autopilot - removed

Autopilot was removed by [ADR-0014](../09_decisions/0014-scouts-and-no-unattended-submission.md).
Unattended discovery and matching are implemented as [Scouts](../02_features/10_scouts.md), separate from application modes.
Automatic submission was declined and was never implemented.

## Invariants

- Mode must be visible to the user.
- Policy checks apply in every mode.
- Nothing submits. No JobPilot code path clicks an external application's submit control, in any mode, under any setting.
- Assist does not interact with external application forms at all. Core refuses to prepare an Assist application in the browser, and the desktop does not offer it.
- Review fills a form and leaves the browser window open; the user submits it themselves.
- The system must persist an auditable application status and action history.
