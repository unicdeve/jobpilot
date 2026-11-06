# Browser Automation Architecture

## Direction

Use Go Playwright as the browser automation technology (ADR-0005).
Since [ADR-0034](../09_decisions/0034-the-candidate-writes-and-fills-their-own-application.md) the browser has one job: reading a job page that no registered source can fetch, for import by URL.
It never fills in, routes through or submits an employer's application form; the candidate applies in their own browser.

## Principles

- Google Chrome must be installed by the user.
- Do not store raw website passwords.
- Use a dedicated JobPilot-owned Chrome profile, never the user's daily profile.
- Launch Chrome with its sandbox on: the browser reads untrusted pages (ADR-0033 §6).
- Never bypass CAPTCHAs or anti-bot protections.
- Treat website DOM/content as untrusted input.

## Responsibilities

The port is `ports.BrowserWorker`:

- `Navigate` to the job's URL.
- `DetectBlocker`: report a page that is posing a check rather than showing the posting.
- `ExtractContent`: return the rendered HTML, which `JobDiscoveryService.ImportJob` hands to the model to extract the job's fields (`docs/02_features/04_job_discovery.md` AC-007).
- `Close` the worker's tab once the page is read.

## A page that asks for a person

Some pages ask whether the user is a person before showing anything: Cloudflare's challenge page stands in front of every Himalayas posting.
The worker recognises one as ADR-0033 §1 describes: Cloudflare's `cf-mitigated: challenge` response header, the `window._cf_chl_opt` variable only its challenge page defines, a large CAPTCHA frame covering the page, or blocking text such as "performing security verification".
An import that meets one stops and says which site is checking, rather than reading the check as the posting.
JobPilot never touches the check, and never hides that its browser is automated.

## One browser, a tab per read

Chrome allows one instance per profile directory, so `ChromeSession` keeps one browser and hands each worker a page of its own.
It launches Chrome on first use, relaunches it if the user closed it, and closes it when the daemon stops.

## What was removed

Until ADR-0034 the worker also walked from a posting to its application form (ADR-0013), extracted the form's fields, filled them from the profile and from generated answers, and waited for the user to answer a check before carrying on (ADR-0033 §2–§5).
That is gone, with the `prepare-in-browser` and agent-run stop endpoints and the browser runs they recorded.
