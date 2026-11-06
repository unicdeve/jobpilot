# Application Modes

There are no application modes.
Every application works the same way: the candidate writes and sends it themselves, and JobPilot keeps what they need beside them ([ADR-0034](../09_decisions/0034-the-candidate-writes-and-fills-their-own-application.md)).

## Autopilot - removed

Autopilot was removed by [ADR-0014](../09_decisions/0014-scouts-and-no-unattended-submission.md).
Unattended discovery and matching are implemented as [Scouts](../02_features/10_scouts.md), separate from applications.
Automatic submission was declined and was never implemented.

## Assist and Review - removed

Assist generated application content, and Review also filled the employer's form in a browser and left it open for the user to submit.
Both were removed by ADR-0034.
Applications that read as written by AI were being rejected, and postings were asking applicants not to use AI to answer their questions.
Filling forms carried most of the product's fragility for little that copying a profile fact does not do.

## Invariants

- Nothing submits. No JobPilot code path touches an external application form, under any setting.
- No JobPilot code path writes text that goes to an employer: no cover letter, no written answer.
- A suggested answer comes only from the candidate's profile, a deterministic derivation from it, or an answer they wrote and kept, and is never filled in on their behalf.
- The system must persist an auditable application status and action history.
