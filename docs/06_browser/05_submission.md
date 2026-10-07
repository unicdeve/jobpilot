# Submission Boundary

JobPilot never submits applications to an employer, in any mode or configuration ([ADR-0014](../09_decisions/0014-scouts-and-no-unattended-submission.md)).
Review mode leaves the prepared Chrome window open for the user to inspect, complete and submit themselves.
Approval in JobPilot records the review decision; it cannot authorize a browser submit action.

After submitting externally, the user can mark an APPROVED application as SUBMITTED and record later outcomes.
Neither submission success nor an employer's response is inferred from the browser or elapsed time.
See [Application management](../02_features/08_applications.md) for the local state model.

Never bypass CAPTCHA or anti-bot mechanisms.
A preparation failure records its reason and leaves the browser for the user; it is not a submission failure.
