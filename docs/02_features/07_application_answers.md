# Feature: Application Answers

## Purpose

The candidate notes the questions an application form asks and writes their own answers beside the job ([ADR-0034](../09_decisions/0034-the-candidate-writes-and-fills-their-own-application.md)).
JobPilot never writes an answer.
It may suggest one from the candidate's own material, which they can use, edit first, or ignore.

## Suggestion order

Question
→ exact profile answer
→ work authorization, for a right-to-work question about a country the candidate has spoken about
→ deterministic derivation
→ an answer the candidate kept to a similar question
→ no suggestion

A suggestion is worked out each time the question is read, from the profile as it is then, and is never stored or filled in.
A narrative question gets a suggestion only from a kept answer, which the candidate wrote.

## What the resolver will not suggest

A lookup answers a label that *names* a fact.
A label that opens by asking the candidate something — "Are you…", "Do you…", "Will you…" — is asking them to decide, and a decision is not a lookup.
This distinction is not cosmetic: without it, "Are you currently based in any of these countries?" was answered with the candidate's city, because the matcher saw the word "based".

Work authorization is the deliberate exception, because it is a decision the profile genuinely knows.
Even then it answers only about a country the candidate has explicitly spoken about, matched by name against the job's own stated locations.
A sponsorship answer for the wrong country is worse than no answer: it is a claim about someone's right to work, made to an employer, under their name.

## Acceptance Criteria

### AC-001
Known factual questions are suggested an answer from profile data.

### AC-002
No code path generates answer text.

### AC-003
Unknown factual questions are not guessed.

### AC-004
Answers are written and edited by the candidate, and a suggestion is used only when they choose it.

### AC-005
A question is never suggested a fact belonging to a different question.
Email and phone, and each kind of link, are distinct fields resolved distinctly (ADR-0015).

### AC-006
Questions and answers are associated with the application, listed oldest first, and can be removed.

### AC-007
An answer the candidate wrote can be kept, and is then suggested for a similar question on another application.
