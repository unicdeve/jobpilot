# Browser Automation Architecture

## Direction

Use Go Playwright as the initial browser automation technology.

## Principles

- Google Chrome must be installed by the user.
- Do not store raw website passwords.
- Use a dedicated JobPilot-owned Chrome profile, never the user's daily profile.
- Never bypass CAPTCHAs or anti-bot protections.
- Treat website DOM/content as untrusted input.
- Browser preparation is implemented; submission is always the user's action.

## Responsibilities

- Navigate to job/application URLs
- Walk from a job posting to the page that actually holds its application form
- Extract application forms
- Extract page content (e.g. a job posting's rendered HTML, for custom job
  URL import — see `docs/02_features/04_job_discovery.md` AC-007)
- Fill deterministic fields
- Fill approved generated answers
- Detect blockers
- Report structured results

## Reaching the form

The stored URL for a job is rarely the employer's form.
It may be a board's listing page, so the worker looks for an apply control and follows it, at most twice, before deciding what it has reached.
The decision is `ADR-0013`, and it has three parts, in this order.

**The deterministic rules choose.**
`services.PickApplyControl` ranks the page's own links and buttons: a destination on a known applicant tracking system beats a URL path that says "apply", which beats wording that does.
On every board measured so far this answers, and no model is involved.

**The model is the fallback, and it proposes rather than acts.**
When the rules rank nothing, `services.ApplyControlChooser` shows the model the controls the page produced — their text and resolved destination, wrapped as untrusted content — and asks it for a position in that list, or to abstain.
It answers with an index, never a URL, so the browser can only ever be sent somewhere the page already pointed, and its choice passes the same destination checks a deterministic pick does.
Abstaining is a correct answer: the run stops and says so rather than guessing.
A model that is absent, unreachable or incoherent leaves the run exactly where it would have been with no fallback at all.

**The gate decides what was reached.**
Whether a hop happened or not, and whoever chose it, nothing is filled until the page asks about a person — `services.LooksLikeApplicationForm`, at least three distinct candidate details.
This is what stands between the candidate's data and a stranger's form: an earlier version filled a job board's newsletter box with the candidate's email address and reported a prepared application.

Every hop is recorded in the `AgentRun` trail with its destination, whether a model chose it, and whether it left the origin.

## Failure behavior

A detected CAPTCHA, unreadable form or fill failure stops preparation and records the reason.
Unresolved questions stay blank for the user.
Some fields with unreadable machine-generated labels are currently skipped without a reported count; see the browser gaps in [Current state](../00_product/current_state.md).
The browser remains open on success and failure until the user closes it or the daemon shuts down.

## Submission boundary

The worker never clicks the target site's submit control.
It leaves the filled browser window open for the user to review and submit.
ADR-0014 makes this a permanent product invariant, with no mode or setting that permits automatic submission.
