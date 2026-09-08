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
- Extract application forms
- Fill deterministic fields
- Fill approved generated answers
- Detect blockers
- Report structured results

## Failure behavior

If a CAPTCHA, unsupported form, authentication failure, or ambiguous field is encountered, stop safely and report the state to the user.

## Submission boundary

The responsibilities above stop at filling fields and reporting results — this worker never clicks the target site's own submit control. It leaves the browser window open, filled in, for the human to review and submit themselves, satisfying Review Mode's explicit-approval requirement without needing a separate confirmation step in JobPilot itself. Clicking submit automatically (with success/failure detection) is out of scope here — see roadmap Phase 7 ("browser handoff") and Phase 8 (autopilot policy gating).
