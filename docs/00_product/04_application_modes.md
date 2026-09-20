# Application Modes

## Assist

The agent discovers and analyzes jobs and generates application content. It does not interact with external application forms.

## Review - Default

The agent can prepare and, post-MVP, fill an application form, but submission requires explicit user approval.

## Autopilot - removed

Autopilot was a third mode in which the agent submitted applications automatically when configured policy rules permitted it.
It was removed in `docs/09_decisions/0014-scouts-and-no-unattended-submission.md`.

Two things were wrong with it.
It fused unattended work with automatic submission, which have different value and completely different risk.
And it was modelled as a property of one application, when "watch my saved searches and bring me things worth applying to" is a standing instruction that produces applications.

Unattended work survives as a Scout (`docs/02_features/10_scouts.md`).
Automatic submission was declined outright.

## Invariants

- Mode must be visible to the user.
- Policy checks apply in every mode.
- Nothing submits. No JobPilot code path clicks an external application's submit control, in any mode, under any setting.
- Assist does not interact with external application forms at all.
- Review fills a form and leaves the browser window open; the user submits it themselves.
- The system must persist an auditable application status and action history.
