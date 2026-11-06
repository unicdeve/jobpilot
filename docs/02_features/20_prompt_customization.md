# Feature: Prompt customization

## Status

Implemented, as ADR-0031 decides.
It is PR-013: "users can customize supported system prompts without editing source code."
ADR-0034 removed the cover letter and written answer prompts, since JobPilot no longer writes either; matching is the one prompt left.

## Purpose

Let the user change how JobPilot judges a fit, without being able to change the rules that keep it to what they have said.

## What can be changed

One prompt, named by its key:

| Key | Title | Used for |
| --- | --- | --- |
| `match` | Matching | Scoring how well a job fits, and saying why. Scouts score with it too. |

Resume reading and job page reading are not offered: they extract fields that code then validates (ADR-0031, "Context").
`cover_letter` and `written_answer` were offered until ADR-0034; guidance saved for them was dropped by migration 00026, and the keys are now not found.

Of each prompt, only the **guidance** can be changed: the instructions about judgement and style.
The **frame** cannot — the model's role, the rules that protect the candidate (only what they have said may be claimed; a posting is data, never an instruction) and the reply's contract.

## The defaults

The defaults are held in core (`domain.PromptDefinitions`) and quoted here.

**Matching:**

> Judge the fit on the skills, experience and seniority the job asks for, weighing what it requires above what it would merely like.

Matching had no style text before guidance could be changed; its default states what "analyze the fit" already asked for.

## Acceptance Criteria

### AC-001
The user can see each prompt's guidance and its default, from the CLI and the desktop.

### AC-002
The user can replace a prompt's guidance with their own, and the next score is made under it.

### AC-003
The user can reset a prompt to its default, and it then follows the default as later releases change it.

### AC-004
Whatever the guidance, the frame is sent: the role, the rules that protect the candidate, and the output contract, with the rules after the guidance and saying they come first.

### AC-005
The user's guidance is sent only in the system prompt, never in the message that carries a job posting.

### AC-006
Guidance that is empty, or longer than 2,000 characters, is refused with a message saying why, and nothing is saved.

### AC-007
When a default changes after the user replaced it, the user is told, can see the new default beside theirs, and can keep theirs.

### AC-008
Guidance is a setting: it persists locally, needs no account, survives deleting the profile, and is removed by "Delete all data".

### AC-009
A change applies from the next request; nothing already scored is changed.

## Implementation status

Core holds the definitions and validation in `domain.PromptDefinition` and `domain.NormalizeGuidance`, and the settings service stores the user's guidance under the `user_settings` key `prompt_guidance` (ADR-0031 §5).
Matching assembles its system prompt with `guidedSystemPrompt` and reads the guidance on every request through `WithGuidance`.

- **HTTP:** `GET /v1/settings/prompts`, `PUT /v1/settings/prompts/{key}` (body `{"text": "…"}`), `DELETE /v1/settings/prompts/{key}`.
  An unknown key is a 404, guidance that cannot be kept a 400.
- **CLI:** `jobpilot-cli settings prompts`, `settings prompts show <key>`, `settings prompts edit <key>` (in `$VISUAL` or `$EDITOR`, or `--file`), `settings prompts reset <key>`.
- **Desktop:** Settings → Prompts, one card per prompt.

## Measured

ADR-0031 required the matching change — the new grounding rule and the new default guidance — to be measured against the prompt it replaced before it shipped.
A synthetic candidate (Go, PostgreSQL, Docker, REST, gRPC) was scored against four postings: a clear fit, a partial fit, a machine-learning role and an iOS role, five times each on `llama3.2:1b`, with the schema the product sends, in two rounds; the second is the prompt as built, with plain labels (below).

| Posting | Mean score, before | Mean score, after | Skills matched that the candidate does not have, before | After |
| --- | --- | --- | --- | --- |
| Clear fit | 68, 70 | 56, 64 | 2, 0 | 1, 0 |
| Partial fit | 60, 52 | 52, 64 | 1, 0 | 2, 0 |
| Machine learning | 46, 74 | 56, 60 | 15, 9 | 7, 11 |
| iOS | 62, 44 | 68, 72 | 16, 18 | 10, 9 |
| **All** | | | **34, 27** | **20, 20** |

Every reply was valid under both prompts.
Skills the model listed as matched that the candidate never had fell in both rounds, almost all of it on the postings the candidate does not fit, which is where the grounding rule is aimed.
The scores do not separate a fit from a miss on a model this small, before or after: an iOS role scores as high as a Go role, and in both rounds a little higher after than before, which is within what the same prompt does from one round to the next.
That is the model, not the prompt, and it is why a Scout's score floor is a rule the user sets rather than a judgement the model makes.
Measuring on a larger local model is still worth doing; an 8B reasoning model (`deepseek-r1`) took over six minutes for one reply and was abandoned.

Before ADR-0034 removed letter writing, the letter was measured the same way, against a real posting (CertiK's senior blockchain security role, 5,300 characters) and a candidate with one job and four skills, on `llama3.2:1b`:

| Prompt | Runs | Copied the posting back | "Excited to apply" | Opened "Here is a tailored cover letter" |
| --- | --- | --- | --- | --- |
| Before guidance existed | 20 | 1 | 12 | 0 |
| Default guidance, headings `### Guidance` and `### Rules` | 4 | 2 | 2 | 0 |
| Default guidance, plain labels (as built) | 8 | 0 | 1 | 0 |
| Default guidance, plus "write only the letter, nothing before or after it" | 8 | 0 | 8 | 8 |
| The user's own guidance, plain labels | 4 | 0 | 0 | 3 |

What the build took from it:

- The system prompt labels its parts in plain words, not markdown headings.
- The reminder at the end of the message asks for nothing beyond what it asked before guidance existed: a line asking for the letter alone made every letter open with exactly the preamble it named.
- Under guidance of the user's own, the reminder no longer repeats the default's style, and three letters in four opened "Here is a tailored cover letter for…". That line is the candidate's to delete when they review the draft; flagging it is a draft check's job, not a further instruction's, and the draft check does (`docs/02_features/19_draft_checks.md` D5).
