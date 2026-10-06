# Feature: About You — Personal Cover Letters and Answers

## Status

Implemented.
D1 to D7 were decided on 2026-09-29, and ADR-0022 records how answers are stored.
Migration `00018` creates `personal_answers` and moves `candidates.application_preferences` into it.
Roadmap: this comes before prompt customization (PR-013); see "Prompt customization" below for why.

## Purpose

Let the candidate tell JobPilot who they are — what they want next, what they value, the work they are proudest of, how they sound — so that a cover letter and a written answer on an application form say something only this candidate could have written.

A letter today is built from the resume and the profile, and nothing else.
The model is told, rightly, never to invent anything about the candidate, so where it has nothing personal to say it writes the stock phrases every letter contains, which is what makes it read as written by AI.
The fix is not a better instruction.
It is information only the candidate has, asked for, so that what the model does not know becomes a question to them instead of a sentence it makes up.

## What exists today

What each generator is given, read from the code:

| | Cover letter (`CoverLetterService.Generate`) | Written answer (`AnswerService.generateAnswer`) |
| --- | --- | --- |
| Job posting | yes, as untrusted data | yes, as untrusted data |
| Profile summary, skills, work history | yes | yes |
| Resume text | the application's resume, if it has one | no |
| Tone and length (`application_preferences`) | yes, one free-text field | no |
| Why this candidate wants this job | no | no |
| What they value, their stories, their voice | no | no |

A written answer is generated when an application is prepared in the browser (`BrowserWorkerService`, through `AnswerService.Resolve`) or when the user resolves a question, for questions of type narrative only.
So "Why do you want to work here?" is answered today from the summary, skills and work history, with nothing about why.

`docs/02_features/06_cover_letters.md` already names "user prompt preferences" as an input and "avoid generic filler where relevant evidence exists" as a rule.
This spec supplies the evidence that rule assumes.

## Principles

- **The candidate's words are the only source of anything personal.**
  Motivation, values and stories are facts about a person, as much as an employer is.
  Nothing here is drafted by a model for the candidate to accept: a model may ask a question, and only the candidate answers it.
- **Nothing is required.**
  Every question can be skipped, and a letter is still written without any answers, as it is today.
  Asking more is how letters get better, not a gate in front of them.
- **Answered once, used everywhere.**
  What the candidate says about themselves is kept and reused for every letter and every written answer, until they change it.
- **A writing sample is style, never content.**
  Anything a sample says about the candidate's past is not a fact the model may state; the profile and the answers are.
- **It stays on this machine.**
  Answers are stored locally with the profile, and go only to the configured LLM, which is local by default.

## Behaviour

### About you

A new section of the profile holds the standing questions.
Each has a stable key, a question and the candidate's answer, and each is optional.

The starting set:

| Key | Question |
| --- | --- |
| `looking_for` | What are you looking for in your next role? |
| `values` | What do you care about in your work, and in the people you do it with? |
| `proudest_work` | Tell us about the work you are proudest of: what was the problem, what did you do, and what changed because of it? |
| `known_for` | What do colleagues come to you for? |
| `career_story` | How did you get into this work, and where are you taking it? Include any change of direction or gap you would like explained your way. |
| `never_mention` | Is there anything a letter or an answer should never mention? |

Tone and length, the existing `application_preferences`, move into this section beside them.

Answers are prose, a few sentences each.
A question's key, not its wording, is what a prompt refers to, so the wording can improve without changing what an answer means.

### A writing sample

The candidate may paste something they wrote and liked — an email, a past letter, a post — so letters can sound like them.
It is used for voice only: sentence length, register, how directly they say things.
The prompt says so, and says that nothing in the sample may be stated as a fact.

### Questions for one application

Before a letter is written, the candidate is offered a few questions about this job:

- **Always:** What draws you to this company, or to this role?
- **Proposed from the posting (D3):** up to two questions where the posting asks for something JobPilot has no evidence for.
  For example: "The posting puts weight on developer tooling. Do you have a story about improving how a team works?"

Answers are kept with the application.
The step can be skipped, and "Write one" still writes.
Writing again — "Rewrite" — offers the same questions, prefilled with what was answered, so a second draft can be steered without starting over.

### What a letter is given

Everything the table above lists today, plus:

- the About you answers, by key
- this application's answers
- the writing sample, marked as style only
- `never_mention`, as a constraint rather than material

### What a written answer is given

The same material as a letter, and the resume, which it lacks today.

A form question that asks what the per-application question asks — "Why do you want to work here?", "What interests you about this role?" — is answered from the candidate's own answer to it first, adapted to the field rather than rewritten from nothing (D5).

### Keeping an answer for next time

When the candidate writes or rewrites a narrative answer themselves, JobPilot offers to keep it: "Use this for similar questions".
A kept answer is stored with the question it answered, and a later narrative question is given the kept answers as material (D6).

### Style of the default prompt

The default prompt gains rules a reviewer would otherwise have to enforce by hand:

- no stock openings or closings ("I am excited to apply", "I believe I would be a great fit", "Thank you for your consideration")
- a concrete claim, backed by something from the profile, instead of an adjective ("I cut deploy time from an hour to six minutes", not "I am a results-driven engineer")
- one reason this company, from the candidate's answer, instead of flattery about the company
- short: under 250 words unless the candidate's tone and length say otherwise

The prompt also says, in so many words, that the posting lists what the employer wants and not what the candidate has, and it repeats the rules briefly at its end, where a small model is likeliest to follow them.

These are the defaults prompt customization will later let the candidate change.

### What the model makes of it

Measured on `llama3.2:1b`, the smallest model a candidate might run locally, against a real posting:

- The questions it proposed from the posting were on point ("How do you approach evaluating the performance of an LLM agent?"), in about four seconds.
- Letters used what the candidate said — the story, the values, the answers about this job — where before they had nothing personal to use.
- The style rules held only partly. Letters still opened with stock phrases and ran past the length asked for, and one in two claimed years of experience the candidate never gave, taken from the posting's requirements; the rule against that, added after the first run, reduced it without ending it.

The rules are instructions, and hold as well as the model follows them.
A deterministic check that flags stock phrases, length and claims the material does not support, for the candidate reviewing the letter, is the follow-up that would not depend on the model.
It is built: `docs/02_features/19_draft_checks.md`.

## Prompt customization

PR-013 lets the candidate change the system prompts without editing source code, within "documented defaults and safe boundaries" (`docs/01_architecture/05_llm.md`).
It comes after this, for two reasons.

A customizable prompt still has nothing personal to work with: customization changes the instructions, and the problem is the material.
And a custom prompt needs a stable, documented set of inputs to refer to; this spec defines them — the About you keys, the per-application answers, the sample.
Building customization first would mean building it twice.

It is now built: `docs/02_features/20_prompt_customization.md`, ADR-0031.
The style rules above are the default guidance for letters and written answers, which the candidate may replace; the rules about grounding, the posting and never mentioning are not guidance, and stay.

## Decisions

### D1 — How answers are stored

**Decided: B.**

- **A. Typed columns on the candidate**, one per standing question.
  Simple to read, but every new question is a migration, and kept answers (D6) and per-application answers need tables of their own anyway.
- **B. Rows of key, question and answer**, with a scope: the candidate, or one application.
  The starting questions are keys the code knows; a kept answer is a row whose question is the form's label.
  One shape for all three, and a prompt reads them the same way.
- **C. One free-text "about me" field.**
  The least work, and it is the shape ADR-0015 spent a change removing: nothing can tell which part answers which question.

ADR-0015 normalized the facts a form looks up, so a resolver never guesses which part of a blob is the phone number.
These are not looked up; they are material for prose.
B keeps each answer attached to the question it answers, which is what ADR-0015 was protecting, without a column per question.
It is a lasting decision about the candidate model, so it should be an ADR.

### D2 — The writing sample

**Decided: A.**

- **A. Accept one, optional.**
- **B. Accept several.**
- **C. None.**

One sample carries the voice; several multiply what the model might lift as fact, and what a small local model has to hold in its context.

### D3 — Questions proposed from the posting

**Decided: B.**

- **A. The fixed question only.**
- **B. The fixed question, plus up to two proposed by the model from the posting.**

The posting is untrusted input, but here it only shapes a question the candidate reads and chooses to answer; nothing it says reaches an employer or an action.
A proposed question is shown as plain text, capped in length, and is never answered by the model.
Without a model configured, or when proposing fails, the fixed question is offered alone.

### D4 — Where the candidate is asked

**Decided: B.**

- **A. In the profile only.**
- **B. In the profile, and at the moment it matters:** the first "Write one" with nothing answered says "Letters stay generic until you tell JobPilot about yourself — about five minutes", with a link to About you, and the per-application questions appear before every letter.
- **C. As well as B, a step in first-run setup.**

Setup is deliberately four things, none of them optional-and-long; a candidate asked for their proudest work before they have seen a single job is asked too early to answer well.

### D5 — A form question the candidate has already answered

**Decided: B.**

When a form asks "Why do you want to work here?" and the candidate answered "What draws you to this company?" for this application:

- **A. Fill the candidate's answer as it is.**
- **B. Give it to the model as the primary material, to fit to the field's wording and length.**

The candidate's words are kept where they fit.
Forms word the question differently and cap the length differently, and a verbatim paste of a 200-word answer into a 500-character field is worse than an adapted one.
The answer shows as generated, and editable, as every generated answer does today.

### D6 — Keeping answers the candidate wrote

**Decided: B.**

- **A. Do not keep them.**
- **B. Offer to keep one, per answer**, and use kept answers as material for later narrative questions.
- **C. Keep every answer the candidate writes, automatically.**

An answer written for one employer may name them or say something only true of that application; keeping it should be the candidate's choice.

### D7 — Applications prepared without the candidate

**Decided: A.**

A scout creates applications without asking anything, and preparing an application in the browser generates its written answers in one pass.

- **A. Use what is already answered, and ask nothing more.**
  Unanswered per-application questions are simply absent.
- **B. Stop preparation to ask.**

Preparation is already a step the candidate reviews before submitting; the per-application questions stay available on the application, and answering them and resolving a question again uses them.

## Deletion

About you answers and the writing sample belong to the candidate, and go when the profile is deleted.
Per-application answers belong to their application, and go when it is deleted.
Kept answers belong to the candidate.
These are new record kinds for ADR-0021's rule table, and its agreement test holds the schema to them.

## Where it is offered

| | Desktop | CLI |
| --- | --- | --- |
| About you, the writing sample | Profile → About you | `profile about` (asks each question in turn), `profile about set <key>` |
| Per-application questions | before "Write one" and "Rewrite" on the application | `application cover-letter generate` asks them first; `--no-questions` skips |
| Keeping an answer | on a narrative answer the candidate edited | `application question keep <question-id>` |

## Acceptance Criteria

### AC-001
The candidate can answer, change and clear each About you question, and add a writing sample; none is required.

### AC-002
A letter is written from the About you answers, the application's answers and the sample's voice, as well as what it uses today.

### AC-003
Before a letter is written, the candidate is offered the per-application questions, and can skip them.

### AC-004
No answer to an About you question or a per-application question is ever written by the model.

### AC-005
A written answer on a form is given the same material as a letter, and the resume.

### AC-006
A narrative answer the candidate wrote can be kept, and is then material for later narrative questions.

### AC-007
Nothing the writing sample says is stated as a fact about the candidate.

### AC-008
Deleting the profile deletes the About you answers, the sample and kept answers; deleting an application deletes its answers.

### AC-009
Without a model configured, the questions are still offered, and only proposing questions from the posting is skipped.
