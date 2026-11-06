# Submission Boundary

JobPilot never submits applications to an employer, under any configuration ([ADR-0014](../09_decisions/0014-scouts-and-no-unattended-submission.md)).
Since [ADR-0034](../09_decisions/0034-the-candidate-writes-and-fills-their-own-application.md) it never fills in an employer's form either: "Open application page" opens it in the user's own browser, and the user writes, completes and submits it themselves.

After submitting externally, the user can mark a MATCHED or APPROVED application as SUBMITTED and record later outcomes.
Neither submission success nor an employer's response is inferred from the browser or elapsed time.
See [Application management](../02_features/08_applications.md) for the local state model.

Never bypass CAPTCHA or anti-bot mechanisms.
