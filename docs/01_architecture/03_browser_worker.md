# Browser Automation Architecture

## Direction

Use Go Playwright as the initial browser automation technology.

## Principles

- Chrome/Chromium must be installed by the user.
- Do not store raw website passwords.
- Prefer existing browser sessions/profile mechanisms where practical.
- Never bypass CAPTCHAs or anti-bot protections.
- Treat website DOM/content as untrusted input.
- Browser automation is post-MVP.

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
On both current connectors it is a board's listing page, so the worker looks for the way in and follows it, at most twice, before deciding what it has arrived at.
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

If a CAPTCHA, unsupported form, authentication failure, or ambiguous field is encountered, stop safely and report the state to the user.

## Submission boundary

The responsibilities above stop at filling fields and reporting results — this worker never clicks the target site's own submit control. It leaves the browser window open, filled in, for the human to review and submit themselves, satisfying Review Mode's explicit-approval requirement without needing a separate confirmation step in JobPilot itself. Clicking submit automatically is not out of scope here, it is out of scope everywhere: ADR-0014 makes "no JobPilot code path clicks an external application's submit control" a product invariant, so there is no phase in which this worker grows that capability.
