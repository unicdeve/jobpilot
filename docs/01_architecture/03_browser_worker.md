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
