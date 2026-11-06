# Feature: Draft checks

## Status

Decided, and implemented.
Since [ADR-0034](../09_decisions/0034-the-candidate-writes-and-fills-their-own-application.md) JobPilot writes no letters or answers, and the checks run over what the candidate wrote: the stock phrases are the ones that make a letter read as anyone's, or as written by a model, and an unsupported claim of years is worth catching whoever wrote it.
The preamble check (D5) and the length taken from writing preferences (D2) were removed: a preamble is a model's note, and writing preferences were an instruction to the model.
A letter is held to 250 words.
D4, the time a model has to write, still applies to matching.
The rest of this page is the original design.
D1 to D4 were decided on 2026-10-02, each as recommended.
D5, the preamble, was added on 2026-10-06.
It is the follow-up `docs/02_features/16_about_you.md` names in "What the model makes of it".

## Purpose

Point out to the candidate, while they review a cover letter or a written answer, what the model was told not to do and did anyway: a line presenting the letter instead of writing it, a stock phrase, a letter past the length asked for, a number of years the candidate never gave.

The prompts already ask for none of these.
A small local model follows them only partly — measured on `llama3.2:1b`, letters still opened with stock phrases, ran past the length, and one in two claimed years of experience taken from the posting.
A rule a model may ignore needs a check that does not depend on the model.

## Principles

- **The check is deterministic and in core.**
  It is ordinary code over the text and the candidate's material, with no model involved, as AGENTS.md asks of every business rule.
- **A flag is advice for the reviewer, never a gate.**
  Nothing is blocked, rewritten or removed.
  The candidate reads the flag and decides; they write and send the application themselves.
- **A claim is unsupported until something the candidate gave supports it.**
  Principle 6 — never invent candidate facts — read the other way round: a number the candidate never stated is one the model invented.
- **Say what was found and why.**
  Each flag quotes the words it is about and gives its reason in a sentence, so the candidate can judge it without knowing how the check works.

## What is checked

| Check | Letters | Generated answers |
| --- | --- | --- |
| Preamble | Yes | Yes |
| Stock phrases | Yes | Yes |
| Length | Yes | No: an answer is "as brief as the question calls for", which no number captures |
| Years of experience | Yes | Yes |

A generated answer is one whose category is `generated`.
A factual answer comes from the profile, not the model, and is not checked.

## Decisions

D1 to D4 were decided on 2026-10-02; each took the recommended option, marked below.

### D1. When a claim of years is supported

- **A. Stated or measured.** (decided)
  A claim that names one of the candidate's skills is supported when the candidate gave that skill a duration and the claim does not exceed it.
  A skill named without a duration is not estimated from the career total, which `16_about_you.md` already rules out for answers.
  A claim that names no skill is supported when it does not exceed the span of the candidate's dated work history (`Candidate.TotalYearsOfExperience`).
  Either way, a claim is also supported when the candidate's own words — resume, profile, About you — say the same number of years about the same thing.
- **B. Verbatim only.**
  Supported only if the same phrase appears in the candidate's material.
  Simpler, and it flags a true "four years" the profile measures but never writes down.
- **C. Career span only**, ignoring per-skill durations.

### D2. The length a letter is held to

- **A. A number in the writing preferences, otherwise 250 words.** (decided)
  "Under 200 words", "about 150 words" or "150–200 words" in the preferences set the limit, the upper end for a range; preferences that name no number leave the prompt's 250.
  A letter is flagged only when it is over the limit, with its count.
- **B. Always 250.**
- **C. A structured length setting** in About you, used by the prompt and the check.
  A schema change, for a preference the candidate already writes in words.

### D3. How flags reach the reviewer

- **A. Computed on read.** (decided)
  Every letter and generated answer the API returns carries its flags, worked out from its current text, so an edit is re-checked at once and nothing is stored.
  The desktop shows them under the text, with the flagged words marked in it.
- **B. Stored at generation.**
  An edit would not be re-checked.
- **C. Letters only, for now.**

### D4. The time a model has to write

The LLM clients gave every request 30 seconds.
A reasoning model run locally, measured with `deepseek-r1`, does not finish a letter inside that, so the request failed while the model was still writing.

- **A. Stream, and give up only when nothing arrives for a while.** (decided)
  Generation is read as it is produced, from Ollama and from OpenAI-compatible servers, and fails only after 60 seconds without a byte, with no limit on the total.
  Before the first byte it waits up to five minutes, since a large model loading from disk sends nothing until it is ready.
  A stream that ends before the model says it is done is an error, not a shorter letter.
  An OpenAI-compatible server that answers in one piece rather than streaming is read that way.
  Cancelling the request still stops it.
  The connectivity check keeps a short limit of its own.
- **B. A longer fixed limit**, such as 10 minutes.
  A stalled server would only be noticed at the limit.
- **C. A limit per provider in the LLM settings.**

### D5. A note from the model before the draft

The system prompt asks for the letter alone, with no commentary around it.
A small model often writes one anyway, as the draft's first line: "Here is a tailored cover letter for the Senior Backend Engineer position at CertiK:".
Asking it not to in the prompt made it worse, not better (`20_prompt_customization.md`, "Measured"), so it is caught here instead.

Measured on `llama3.2:1b` on 2026-10-06 through `CoverLetterService.Generate`, eight letters in each of four conditions, for a posting of about 30 lines and a candidate with two jobs:

| Guidance | Resume | Opened with a preamble |
| --- | --- | --- |
| Default | None | 4 of 8 |
| Default | With a contact line and headings | 0 of 8 |
| The candidate's own | None | 8 of 8 |
| The candidate's own | With a contact line and headings | 5 of 8 |

Every one was "Here is" or "Here's" a tailored cover letter — for the role, for "the job", for "the candidate" or for "the job and candidate" — on a line of its own ending in a colon.
None closed with commentary, such as an offer to revise.
Under the default guidance with a resume, five letters in eight were not letters at all but resumes with `###` headings, which is why that row has none.

- **A. The first line only, as a flag of its own.** (implemented)
  A draft whose first line presents a letter, answer, response or draft — "Here is", "Here's" or "Below is", then "a", "an", "the" or "your", optionally after "Sure", "Certainly", "Of course" or "Okay" — is flagged, quoting the line up to its colon.
  "Here is my letter" is in the candidate's voice and is not flagged.
  The rest of the draft is checked without the line, so it does not count towards the letter's length.
  Its reason is not a stock phrase's: it is not weak writing but no part of what is sent.
- **B. As a stock phrase.**
  The reason would be wrong, and the line would count towards the length.
- **C. Opening and closing.**
  No measured letter closed with commentary, and a closing such as "Let me know if you have any questions" is one a candidate writes.

## How a claim is read

A claim is a number of years — in digits or words up to twenty, with or without "+", "over" or "more than" — in a sentence that speaks in the first person or mentions experience.
"Two years ago", "in the next five years" and "a ten-year-old company" are not claims.
The claim is about a skill when the sentence names one of the candidate's skills, and about their career otherwise.
"Over eight years" claims at least eight.

A duration is compared after rounding to the nearest whole year, the way a person would say it: 4.6 years supports "five years", 4.4 does not.

## Acceptance criteria

### AC-001
Given a letter or answer containing a phrase from the stock list, in any capitalization and with "I am" or "I'm", when it is read, then it carries a stock-phrase flag quoting the phrase.

### AC-002
Given a letter, when it is read, then it is flagged only if its word count is over 250.
An answer is never flagged for length.

### AC-003
Given a claim of *n* years about a skill the candidate gave a duration for, when it is read, then it is flagged only if *n* exceeds that duration rounded to the nearest year.

### AC-004
Given a claim of *n* years about a skill the candidate gave no duration for, when it is read, then it is flagged, unless the candidate's own words say *n* years in a sentence naming that skill.

### AC-005
Given a claim of *n* years that names no skill, when it is read, then it is flagged if the candidate has no dated work history or *n* exceeds its span rounded to the nearest year, unless the candidate's own words say *n* years.

### AC-006
Given a number of years that is not a claim — a time ago, a time to come, an age — when it is read, then it is not flagged.

### AC-007
Given a letter the candidate edits, when it is read again, then its flags are those of the edited text.

### AC-008
Given a model that writes for longer than 30 seconds while still producing output, when it generates, then the reply is returned; given a server that stops sending for 60 seconds, or sends nothing for five minutes, then the request fails saying so.

## Out of scope

- Claims other than years: an employer, a tool or an accomplishment the material does not contain.
  Telling those apart from the posting's own words needs more than a pattern.
- Rewriting anything a flag points at.
- Commentary after the draft, until a model is measured writing it.
