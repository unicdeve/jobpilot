# Feature: Application Answers

## Question categories

1. Deterministic profile facts
2. Derived factual answers
3. Generative narrative answers
4. Unknown/unsafe questions requiring user input

## Resolver order

Question
→ exact profile answer
→ work authorization, for a right-to-work question about a country the candidate has spoken about
→ deterministic derivation
→ generated answer, for narrative questions only
→ ask the user

## What the resolver will not answer

A lookup answers a label that *names* a fact.
A label that opens by asking the candidate something — "Are you…", "Do you…", "Will you…" — is asking them to decide, and a decision is not a lookup.
This distinction is not cosmetic: without it, "Are you currently based in any of these countries?" was answered with the candidate's city, because the matcher saw the word "based".

Work authorization is the deliberate exception, because it is a decision the profile genuinely knows.
Even then it answers only about a country the candidate has explicitly spoken about, matched by name against the job's own stated locations.
A sponsorship answer for the wrong country is worse than no answer: it is a claim about someone's right to work, made to an employer, under their name.

A question that offers a fixed set of choices is answered only with one of those choices, matched exactly.
"Yes" is not evidence for "Yes, I have a work visa", and an answer the field does not offer is left for the user rather than forced into it.

## Acceptance Criteria

### AC-001
Known factual questions use profile data.

### AC-002
Generative questions use job and candidate context.

### AC-003
Unknown factual questions are not guessed.

### AC-005
A question is never answered with a fact belonging to a different question.
Email and phone, and each kind of link, are distinct fields resolved distinctly (ADR-0015).

### AC-006
A question offering a fixed set of choices is either answered with one of them or left for the user.

### AC-004
Generated answers are editable.

### AC-005
Answers are associated with the job/application.
