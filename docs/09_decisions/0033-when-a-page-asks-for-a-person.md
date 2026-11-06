# ADR-0033: When a page asks for a person

## Status

Superseded by ADR-0034 (`0034-the-candidate-writes-and-fills-their-own-application.md`) on 2026-11-06 for preparing an application, which JobPilot no longer does.
Part 1 still applies to importing a job from a URL, which stops on a page that is a human check rather than reading it as the posting, and part 6 still applies to the browser that reads it.

Accepted on 2026-10-24.
The user approved four parts together: recognise a page that is itself a human check, pause the run while the user solves it, hand off to the user's own browser when the site will not let JobPilot's through, and keep Chrome's sandbox on.

It extends ADR-0013 §4, which taught the worker to tell a CAPTCHA that is present from one that is blocking, and changes what happens after a blocking one is found.
It does not change the rule that JobPilot never gets past an anti-bot measure itself (AGENTS.md, `docs/01_architecture/03_browser_worker.md`).

## Context

Preparing a Himalayas job failed with "could not find a way to the application form on this page — no link or button on it looks like an apply control".
The page was not a job posting at all.
It was Cloudflare's challenge page, which stands in front of every Himalayas posting.

Measured on 2026-10-24, loading a Himalayas posting in Chrome driven by Playwright:

- The response is `403` with `cf-mitigated: challenge`.
- The title is "Just a moment...", and the visible text is "Performing security verification", the site's name, a Ray ID and two links.
- The page defines `window._cf_chl_opt`.
- The Turnstile checkbox is a frame from `challenges.cloudflare.com` inside a closed shadow root, so a CSS selector finds no frame at all, and its "Verify you are human" text is not part of the page's own text.

`DetectBlocker` looked for a large CAPTCHA frame by CSS and for three phrases in the page's text, and this page has neither.
The run carried on, found only "Privacy" and "Cloudflare" to click, and reported a missing apply control.

The user then solved the check in JobPilot's window, and it came back.
In their own Chrome it passes once.
Cloudflare scores the browser as well as the click, and Playwright's Chrome says it is automated: it is launched with automation switches, sets `navigator.webdriver`, and was launched with `--no-sandbox`, which Chrome announces in a banner.
A person ticking the box does not change what the browser reports about itself.

The only way to make that check pass in JobPilot's window is to hide that the window is automated.
That is circumventing an anti-bot protection, which the product rules out (`docs/00_product/06_non_goals.md`).
Himalayas' API offers no other way in: its `applicationLink` is the same protected posting page, and a plain HTTP request for it is refused with `403`.

Two other things were true before this decision.
A blocked run moved the application to FAILED at once, so a check the user could have solved in a few seconds — a reCAPTCHA challenge on an employer's form, say — ended the run anyway.
And preparing ran inside one HTTP request, so the run could not wait for anything without holding the request open.

## Decision

### 1. A page that is the check is a blocker, however small its checkbox

A challenge frame that covers a form stays a blocker, and a checkbox beside a working form stays not one (ADR-0013 §4).
A page that *is* the check is now recognised too, by any one of:

- the page's last document response carrying `cf-mitigated: challenge`, which is Cloudflare's documented signal;
- the page defining `window._cf_chl_opt`, which only Cloudflare's challenge page does;
- "performing security verification" in the page's visible text, beside the phrases already listed.

The reason names the site: "himalayas.app is checking that you're a person".

### 2. A check pauses the run, and the user solves it

A blocker no longer ends the run.
The run's status becomes `waiting_for_user`, the step is recorded, and the worker brings the run's tab to the front.
The run then checks the page once a second.

- **The check clears** when the page shows no blocker twice in a row, a second apart, so a page between two documents does not read as solved.
  The run returns to `running`, records that the check was solved, and carries on from the page it is on.
  That may be a page the user navigated to themselves while the window was theirs, which is fine: the gate still decides whether it is an application form.
- **The user stops it** from the desktop or the CLI.
  The run ends as blocked, "you stopped this run".
- **Five minutes pass.**
  The run ends as blocked, saying the check was not solved in time.

Nothing here touches the check.
The user solves it, in a window they can see, and JobPilot only looks at whether it is still there.

The desktop is the place the user is told, with the run's status on the application.
No operating-system notification is posted.
ADR-0028 §2 decided that a task the user started and is watching does not notify, and a check appears seconds after the user presses Prepare, while they are looking at it.

### 3. A check that comes back means the site refuses JobPilot's browser

If the page reloads while the check is being waited on and the check is still there, the site has rejected the solved check.
If the check clears and then reappears on the same page before the run moves on, the same is true.
Either way the run stops at once rather than asking the user to solve it again for five minutes: "himalayas.app asked again after the check was solved — it does not let JobPilot's browser through".

That is the site's decision about automated browsers, and JobPilot respects it.
The desktop then offers the hand-off in §4.

### 4. The user can start a run from a link of their own

An application gains an optional apply link, `apply_url`.
When it is set, a run starts there instead of at the job's own link.
It is a fact the user gave, so it is kept on the application rather than on one run, and "Prepare again" uses it until the user clears it.
`PUT /v1/applications/{id}/apply-url` sets it, and an empty link clears it.

When a run stops blocked, the desktop offers the way round that needs no automation:

1. **Open the posting in your browser**, the user's own default browser, where the check passes once.
2. Click the posting's apply button there, and copy the address of the application form it leads to.
3. **Prepare from this link**: JobPilot stores the link and runs from it.

The employer's form is usually on an applicant tracking system — Ashby, Greenhouse, Lever — which does not stand behind a challenge page, and the rest of ADR-0013 applies unchanged from there.
The link must be an absolute `http` or `https` address, the same rule every hop is held to (ADR-0013 §2).
It is the user's own input, not the page's, so the untrusted-content rules for page links do not apply to choosing it.
They still apply to everything on the page it leads to.

### 5. Preparing runs in the background

`POST /v1/applications/{id}/prepare-in-browser` checks what it can at once — the application exists, is not in Assist mode, has a job, has no run already in progress — records the run, and answers `202 Accepted` with it.
The run belongs to the daemon from then on, as a Scout's does (ADR-0027), so it outlives the request and the desktop can show it waiting.

- Starting moves an application that may enter PREPARING into it: a matched one, or a failed one being tried again.
  Before, a failed application had to be moved back by hand, and "Prepare again" filled the form and then failed on the final transition — an error the request reported, and a background run would have lost.
  An application in any other state is refused with `409 Conflict` before a run is recorded.
- A second start while one is in progress answers `409 Conflict`.
- `POST /v1/agent-runs/{id}/stop` stops a run in progress, and answers `409` for one that is not.
- The desktop follows a run in progress by reading the application's runs every two seconds until none is in progress.
- The CLI follows it the same way, prints when it is waiting for the user, and stops the run if the user interrupts the command.
- A run in progress when the daemon stops ends as blocked, "JobPilot stopped".
  A run the daemon could not close — it was killed — is closed when the workspace next opens, as failed, "JobPilot stopped while this run was in progress".
  Runs only exist inside a running daemon, so a run still open at that point is one nothing will ever finish.

### 6. Chrome runs with its sandbox

Playwright launches Chrome with `--no-sandbox` unless told otherwise.
JobPilot's browser reads untrusted pages, so it is launched with Chrome's sandbox on.
This is the browser's own security, not a way round any check, and it does not make Cloudflare's check pass.

## What this does not do

- No change to what the browser reports about itself: no hidden `navigator.webdriver`, no removed automation switches, no changed fingerprint.
- No CAPTCHA-solving service, and no clicking of a check on the user's behalf.
- No carrying a clearance cookie from another browser into JobPilot's, which would be the same circumvention by another route.

## Consequences

- A run that meets a check the user can solve finishes instead of failing.
- Himalayas postings still cannot be prepared from the posting itself.
  They take one step in the user's own browser, and are then prepared from the employer's form, which is what the run was trying to reach anyway.
- The run status set gains `waiting_for_user`.
  The activity screen and its filter show it.
- `ports.BrowserWorker` gains `Focus`, to bring a run's tab forward, and `PageLoads`, a count core compares to tell whether the page reloaded.
- `applications` gains a nullable `apply_url` column.
- The prepare endpoint's response changes from the finished application to the started run.
  The desktop and the CLI change with it.
