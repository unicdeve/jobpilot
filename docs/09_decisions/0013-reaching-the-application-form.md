# ADR-0013: Reaching the application form

## Status

Accepted, and implemented in full.
The user chose option C: heuristics decide where to go, and the LLM is consulted only when they find nothing.

One detail of part 3 below was changed by what the real pages turned out to look like, and the code follows the pages.
The file-input arm of the gate is not implemented: a resume upload is an `<input type=file>`, which the worker deliberately skips because it cannot be filled from a text answer, so it never reaches the gate to count.
The three-distinct-fields arm decides alone, and all three recorded ATS forms clear it comfortably.

ADR-0033 extends part 4: a page that is itself a human check is a blocker however small its checkbox, and a blocker now waits for the user to answer it rather than stopping the run.

## Context

`BrowserWorkerService.PrepareApplication` navigates to `job.CanonicalURL` and scrapes every `input`, `select` and `textarea` on whatever page loads.
It clicks nothing.
There is no second navigation.

On both current connectors the canonical URL is a job board's listing page, not an employer's form, so the step does not merely find nothing — it finds the wrong thing.
Observed live on an Arbeitnow posting: the only field on the page was the board's own "Enter your email" job-alert signup, which the resolver answered with the candidate's contact address, the worker filled, and the run reported as `completed` before moving the application to READY_FOR_REVIEW.

Measured on the user's own stored Arbeitnow posting (lemlist, September 2026), the route and both pages look like this:

- The listing page carries an `<a href=".../apply">Apply Now</a>` and a matching button, and two inputs: the board's newsletter `emailAddress` field and its submit — both inside a `<form>`.
- `/apply` redirects to `jobs.ashbyhq.com/<company>/<uuid>/application`, cross-origin, on the second hop.
- That page has 23 fields — Name, Email, Resume (file, required), Phone, LinkedIn, GitHub, salary expectations, availability, several radio groups — and **no `<form>` element at all**.
- It also loads an invisible reCAPTCHA iframe while remaining entirely fillable.

Real postings reach their form in one of three shapes:

1. The form is on the posting itself (rare on aggregators, common on company career pages).
2. An "Apply" control links to the employer's ATS on another origin — Ashby, Greenhouse, Lever, Workday, SmartRecruiters.
3. An "Apply" control opens the form in place, behind JavaScript, with no navigation at all.

Today only shape 1 can work, and only by luck.

## Decision drivers

- A wrong page must never look like a prepared application (the live defect above).
- Job postings and the pages they link to are untrusted input (AGENTS.md core principle 8).
- The worker must never submit, and must never defeat an anti-bot measure.
- Deterministic business rules must not be delegated to an LLM (core principle 7).
- Long-term maintainability matters more than the cost of building it: an ATS-per-connector rule set that nobody updates is worse than nothing.

## Options

### A. DOM heuristics only

Look for an anchor or button whose text, `href` or ARIA label matches a known apply vocabulary, follow it, and repeat once if the destination is a known ATS host.

- Predictable, auditable, cheap, offline, and easy to test against fixtures.
- Brittle in exactly the places that matter: "Apply now", "Bewerben", "I'm interested", an icon-only button, a form behind a JS modal.
- Every new board is a new rule, and a missed rule is silent.

### B. An LLM decides where to go

Give the model the page's converted markdown and ask which control leads to the application form.

- Handles wording, language and layout variation, which is most of the problem.
- Puts a model in the navigation loop, so a crafted posting could aim the browser at a page of its choosing — the "treat external webpages as untrusted" principle applies directly.
- Non-deterministic: the same posting may resolve differently between runs, which makes failures hard to reproduce.

### C. Heuristics first, LLM as a fallback, with a deterministic gate on the result

Try A.
If it finds nothing, ask the model to name a candidate control from the page's own extracted links, restricted to controls that actually exist in the DOM.
Follow it only if the destination passes deterministic checks, then require the destination to look like an application form before anything is filled.

- Keeps the common case deterministic and debuggable.
- The model proposes, it does not act: it may only choose among controls already extracted from the page, so it cannot invent a URL.
- The gate is the part that fixes the observed defect, and it protects options A and B equally.

## Decision

Option C, in three parts.

### 1. Find the apply control

A deterministic pass over the page's own anchors and buttons, ranked by an apply vocabulary and by whether the destination host is a known ATS.
If it produces exactly one strong candidate, use it with no model involved.

If it produces none, the model is given the list of controls already extracted from the DOM — text and resolved href only — and asked to choose one or abstain.
It returns an index into that list, never a URL, so the browser can only ever go somewhere the page already pointed.

### 2. Constrain the navigation

- At most two hops from the posting, so a redirect chain cannot wander.
- `http`/`https` only.
- The destination is recorded in the `AgentRun` trail as its own event, with the reason it was chosen and whether a model was involved.
- Cross-origin hops are allowed (the ATS is the point) but recorded as such.

### 3. Refuse to call the wrong page an application form

This is the gate, and it applies whether or not a hop happened.

The gate is about **what the fields are**, never about whether a `<form>` element wraps them.
Measured on the two real pages in the Context above, that signal is exactly inverted: Arbeitnow's newsletter box sits inside a `<form>`, and Ashby's genuine 23-field application has no `<form>` element anywhere on the page.
A single email field does not qualify either — an email input inside a form is the most common newsletter on the web, and it is precisely what was filled in the observed defect.

A page counts as an application form when it holds either a file input asking for a resume/CV, or at least three distinct candidate-ish fields (name, email, phone, location, cover letter, links).
Otherwise the run stops with "no application form found on this page", which is a correct and useful outcome.
Filling a newsletter box is not.

A stopped run moves the application to FAILED with the reason, which is what the state machine already does for a CAPTCHA or an unreadable form.

### 4. Distinguish a CAPTCHA that is present from one that is blocking

`DetectBlocker` currently flags any frame whose URL contains `recaptcha`, `hcaptcha`, `turnstile` or `arkoselabs`.
The real Ashby application page loads `recaptcha.net/recaptcha/api2/anchor` as an invisible reCAPTCHA while the form is perfectly fillable, so as written this rejects every Ashby posting the moment the worker can finally reach one — trading a silent wrong success for a blanket false failure.

Presence is not a challenge.
The check must look for a challenge the user is actually being asked to solve (a visible challenge frame, or the blocking phrases already listed), not for the vendor's script being on the page.
This is a prerequisite of the rest of the decision, not a follow-up to it.

## Consequences

- `ports.BrowserWorker` gains the ability to list a page's candidate controls and to click one; `PrepareApplication` gains a find-and-follow step before extraction, and a gate before filling.
- On the one board measured, heuristics alone find the control ("Apply Now", `href` ending `/apply`), so the LLM fallback is not reached at all. That is the expected shape: the model is there for the boards that do not read so plainly, not for the common case.
- The LLM enters the browser loop for the first time, in a proposal-only role, and `docs/01_architecture/03_browser_worker.md` needs a section saying exactly that.
- Extraction quality on real ATS pages becomes measurable: fixtures for Ashby, Greenhouse and Lever, plus the two boards' listing pages as negative cases that must be refused.
- Some postings will now fail that previously "succeeded". That is the point: they were never prepared.
- Login walls become more visible, since ATS pages often want an account. That is the existing documented gap, not a new one.

## Settled by default

Two details were put to the user and not separately answered, so the decision above stands as written and they are recorded here rather than left implicit.

- A cross-origin hop to an ATS happens automatically rather than pausing for approval.
  Note that on the measured route the cross-origin move arrives as a *redirect* from a same-origin click, so an approval prompt would have to hook redirects rather than clicks to catch it at all.
  Following a link is not a consequential external action in the sense AGENTS.md means — nothing is sent, nothing is submitted, and the user already asked for this application to be prepared.
  Every hop is recorded in the `AgentRun` trail, so it is reviewable after the fact.
- Two hops is the ceiling.
  A posting to an ATS is one hop; an ATS landing page to its own form is the second.
  Anything further is more likely a redirect loop or a tracker than a route to a form.

Either can be revisited without disturbing the rest of the decision.
