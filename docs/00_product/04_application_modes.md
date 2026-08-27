# Application Modes

## Assist

The agent discovers and analyzes jobs and generates application content. It does not interact with external application forms.

## Review - Default

The agent can prepare and, post-MVP, fill an application form, but submission requires explicit user approval.

## Autopilot

The agent can submit applications automatically when configured policy rules permit it.

## Invariants

- Mode must be visible to the user.
- Policy checks apply in every mode.
- Assist cannot submit.
- Review cannot submit without explicit user approval.
- Autopilot must still enforce policy restrictions.
- The system must persist an auditable application status and action history.
