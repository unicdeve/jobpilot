# ADR-0034: The candidate writes and fills their own application

## Status

Proposed on 2026-11-06.
The user chose the direction and the answers to its open questions on 2026-11-06; this ADR records them for approval before any code changes.

It supersedes ADR-0007 (application modes) and ADR-0013 (reaching the application form), and ADR-0033 (when a page asks for a person) as it applies to preparing an application.
It narrows ADR-0014, ADR-0022 and ADR-0031, as described under "Decision".
It does not change ADR-0005: Go Playwright stays, for importing a job from a URL.

## Context

JobPilot prepares an application in two ways today.

1. It writes: a cover letter (`CoverLetterService.Generate`) and narrative answers (`AnswerService.generateAnswer`), drafted by the configured model from the profile, the resume, About you and the job posting.
2. It fills: Review mode opens Chrome through Go Playwright, finds the route to the employer's form, extracts the form's fields, resolves an answer for each and types it in, then leaves the window open for the user to submit (ADR-0013, ADR-0033).

Applications that read as written by AI are being rejected, and some postings now say outright that applicants must not use AI to answer their questions.
Draft checks (`docs/02_features/19_draft_checks.md`) and About you (ADR-0022) made generated text sound more like the candidate, but the text is still drafted by a model, and a posting that forbids AI forbids exactly that.
A JobPilot user who follows such a posting today has to discard what JobPilot made for them.

Filling a form from profile facts is not AI writing, but it carries most of the cost and fragility in the product:
routing to the form with a model fallback, form extraction across ATS vendors, the human-check pause and hand-off, and sites that refuse an automated browser outright (Himalayas behind Cloudflare, ADR-0033).
Without the written answers, what is left of the fill is the candidate's name, contact details, links and a few derived facts, which a candidate can paste in seconds.

What users get the most from — discovery, matching, Scouts, Activity and outcome tracking — does not depend on either.

## Decision

### 1. JobPilot does not write application content

No code path asks a model to write a cover letter, a written answer, or any other text that goes to an employer.
The `cover_letter` and `written_answer` prompts, and their customization (ADR-0031), are removed.
The model remains in use for what it judges, not what it writes on the candidate's behalf: matching, resume-to-profile suggestions, and reading an imported job page.

A cover letter is something the candidate writes in JobPilot's editor.
Letters stay versioned and editable, and deterministic draft checks still advise on them, because stock phrases and unsupported years of experience read badly whoever wrote them.

### 2. JobPilot does not fill external forms

Browser preparation is removed: the prepare-in-browser endpoint and command, the apply route and its model fallback, form extraction and filling, the human-check pause and hand-off, and the browser runs they record.
The application page offers "Open application page", which opens the posting's application link, or the user's own saved link, in the user's default browser.
It never opens it in a browser JobPilot controls.

The application page shows the candidate's own material ready to copy: profile facts (name, contact, location, links, work authorization, availability), reusable personal answers, and the About you stories.
Copying is the user's action; nothing is typed into a page.

This makes the rule in AGENTS.md stronger, not weaker: JobPilot still never submits, and now never touches an employer's form at all.

### 3. One flow instead of two modes

With nothing to fill, Assist and Review are the same thing, so the mode field is removed.
The state path becomes MATCHED → APPROVED → SUBMITTED, with outcomes unchanged after SUBMITTED.
APPROVED means "ready to send": the user has written what they need and is about to apply.
DISCOVERED stays as the starting state, and MATCHED → SUBMITTED is also allowed, for a user who applies without recording a review step.
PREPARING and READY_FOR_REVIEW are removed; FAILED is removed from the pre-submission path, since nothing runs that can fail.

### 4. Application questions are the candidate's own notes

A question on an application is one the user adds, with an answer the user writes.
JobPilot may suggest an answer only from a deterministic source: a profile fact or derivation that the resolver already uses (`docs/02_features/07_application_answers.md`), or a saved personal answer whose question matches.
A narrative question gets no suggestion.
The user accepts, edits or ignores a suggestion; nothing is filled in on their behalf.

### 5. About you stays, without the writing sample

About you's stories and reusable personal answers stay: they are the candidate's own words, and they become copy-ready material and the source of answer suggestions.
The writing sample is removed: its only use was steering the model's voice.
Per-application context collected before generating a letter ("what draws you to this role") is removed as a separate store; the user writes it as a question note on the application instead.

### 6. Existing generated content is deleted

A migration deletes every stored cover letter version, every application question and answer, and every browser preparation run and its events.
They were drafted by a model or extracted by the removed browser flow, and the user chose not to keep them.
Applications in PREPARING, READY_FOR_REVIEW or FAILED before submission move to MATCHED, recorded in their history with the reason, so no application disappears and no outcome is invented.
Scout runs, their events, matches and submitted applications with their outcomes are untouched.

## What this does not do

- It does not remove the LLM provider, matching, or Scouts.
- It does not remove Go Playwright, which job import by URL still uses to read a page.
- It does not decide the tailored resume builder, which is planned separately; this ADR only requires that it follow the same rule, that JobPilot does not write candidate content for an employer without a new decision.

## Consequences

- An application JobPilot helps with contains only words the candidate wrote, which satisfies postings that forbid AI-written answers.
- Applying takes the user longer than a filled form did, mitigated by one-click copy of every fact they would type.
- A large and fragile part of the codebase goes away, along with the anti-bot failure modes that came with it.
- Activity shows Scout runs only, until another kind of run exists.
- Users lose previously generated letters and answers when they upgrade; release notes must say so.
