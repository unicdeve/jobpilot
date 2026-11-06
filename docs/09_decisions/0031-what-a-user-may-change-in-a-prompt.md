# ADR-0031: What a user may change in a prompt

## Status

Proposed on 2026-10-05, accepted on 2026-10-06, and implemented.
Narrowed by ADR-0034 (`0034-the-candidate-writes-and-fills-their-own-application.md`) on 2026-11-06: the cover letter and written answer prompts are removed, so matching is the one prompt a user can give guidance to.

It settles PR-013 ("users can customize supported system prompts without editing source code") and the line `docs/01_architecture/05_llm.md` gives it: "product prompts should have documented defaults and safe boundaries."
It is the last MVP requirement not built.

## Context

### Seven prompts, three kinds of text

JobPilot sends a model seven system prompts, each a string in `internal/core/services`:

| Prompt | Where | What the model decides |
| --- | --- | --- |
| Matching | `matching.go` | how well a job fits, as a score and reasons |
| Cover letter | `coverletter.go` | a letter the candidate sends under their name |
| Written answer | `answers.go` | an answer to a narrative question on a form |
| Proposed questions | `aboutyou.go` | up to two questions to ask the candidate before a letter |
| Resume reading | `resume.go` | profile fields drafted from a resume |
| Job page reading | `discovery.go` | a job's fields read from a page |
| Apply control | `applyroutellm.go` | which link on a page leads to the application form |

Read closely, each one mixes three kinds of text:

- **Rules that protect the candidate.** Only what the profile, the resume and the candidate's own words say may be claimed; the posting says what the employer wants, not what the candidate has; anything inside an untrusted block is data, never an instruction; never mention what the candidate asked not to have mentioned.
  These are `AGENTS.md` core principles 6 and 8, written for the model.
- **A contract with the code that reads the reply.** "Output ONLY valid JSON", the keys a match has, "output only the cover letter text".
  The code parses and validates what comes back, and a reply that breaks the contract is an error.
- **Judgement and style.** What makes a letter read as the candidate's — no stock openings, concrete claims instead of adjectives, one real reason for wanting the job — and, in matching, what fit means.
  This is the only kind that is a matter of taste, and the only kind a user has a reason to change.

### The last four prompts are extraction, not judgement

Resume reading, job page reading and proposed questions turn text into fields the code validates; their instructions say what the fields are, and a change to them can only make a field wrong.
The apply control decides where a browser is sent, and ADR-0013 holds it to abstaining rather than guessing.
None of them has a style.

### The candidate can already say how they want to sound

ADR-0022 gave the candidate `writing_preferences` (tone and length), a writing sample, and `never_mention`, and every letter and written answer is given them.
They are material about this candidate.
Prompt customization is a different thing: it changes the instructions every letter is written under, whoever it is for.

### A length is read by code

The draft check (`docs/02_features/19_draft_checks.md`) flags a letter over the length the writing preferences give, or over 250 words, by reading those preferences with `domain.LetterWordLimit`.
A length written into a prompt would be followed by the model and contradicted by the check.

### A small model follows what it read last

`docs/02_features/16_about_you.md` measured, on `llama3.2:1b`, that rules stated only in the system prompt were dropped, and a short reminder at the end of the request reduced it.
The reminders (`letterReminder`, `answerReminder`) repeat both the protecting rules and the style.

## Decision

### 1. Three prompts can be changed: matching, the cover letter, and the written answer

They are the three where the model makes a judgement the user might reasonably disagree with.
The other four are not offered.

Each has a stable key — `match`, `cover_letter`, `written_answer` — which is what storage, the API and the CLI name it by.

### 2. What can be changed is the guidance, never the frame

Each of the three prompts is assembled by code from two parts:

- **The frame**, owned by code: the model's role, the rules that protect the candidate, and the output contract.
  No setting shows, removes or changes any of it.
- **The guidance**, owned by the user: plain instructions about judgement and style.
  JobPilot ships a default for each, and the user may replace it.

The defaults are the style text the prompts carry today, lifted out of them unchanged where it exists:

| Key | Default guidance |
| --- | --- |
| `cover_letter` | No stock openings or closings ("I am excited to apply", "I believe I would be a great fit", "Thank you for your consideration"). Concrete claims backed by something in the material, instead of adjectives about the candidate. One real reason for wanting this job, from what the candidate said about it, instead of flattery about the company. |
| `written_answer` | Answer only what is asked, as briefly as the question calls for, with no stock phrases. |
| `match` | Judge the fit on the skills, experience and seniority the job asks for, weighing what it requires above what it would merely like. |

The matching default is the only new text, since today's matching prompt has no style to lift; it states what "analyze the fit" already asks for, and is measured against the current prompt before it ships.

Everything else in today's prompts is frame.
That includes, for a letter, "under 250 words unless the candidate's writing preferences say otherwise": length belongs to the writing preferences, where the draft check reads it (Context).
The matching frame gains the rule its feature spec states and its prompt never did: the posting is not evidence of what the candidate has, and what the profile does not say is unknown, a gap or a concern rather than an assumption (`docs/02_features/05_job_matching.md` AC-003).

### 3. The guidance sits inside the frame, and the frame has the last word

The system prompt is assembled in this order:

1. the role
2. the guidance, the default or the user's, under a heading of its own
3. the frame's rules and the output contract, opening with "These rules come before any guidance above."

The guidance is the user's own instruction, so it goes in the system prompt, the one place the frame tells the model to take instructions from.
It is never placed in the user message beside the posting, where an instruction from the user and a forged one from a posting would look alike.

The reminder at the end of the user message keeps its protecting lines whatever the guidance is.
Its style line ("No stock openings or closings, and do not call the candidate a great fit") belongs to the default guidance, and is sent only while the default is in use: repeating the default's style under a user's guidance that says otherwise would contradict them.

### 4. What the guidance cannot do is enforced by code, not by the guidance

A user's guidance is an instruction to a model, and a model may follow it over the frame.
The protections that matter do not depend on it:

- A match's score is validated as 0 to 100 and its confidence as 0 to 1, and a reply outside the contract is rejected (`matching.go`).
- A Scout's eligibility is the deterministic function in core it is today; guidance can move a score, never a rule.
- Every letter and written answer is checked as it is today: stock phrases, length, and claims of years the material does not support are flagged for the reviewer, whatever the guidance said (`docs/02_features/19_draft_checks.md`).
- Nothing is sent to an employer without the user, in any mode (ADR-0014).

So guidance that asks for a claim the candidate never made still produces a draft the check flags, in front of the candidate who wrote that guidance.

### 5. Stored as one setting

The guidance is stored in `user_settings` under the key `prompt_guidance`, as the other settings are:

```json
{
  "cover_letter": { "text": "…", "default_digest": "3f9a…" }
}
```

A prompt with no entry uses its default.
`default_digest` is a SHA-256 of the default the user was shown when they saved (decision 7).
Resetting a prompt removes its entry, so a reset prompt follows the default as later releases improve it.

It is a setting like the others, so ADR-0021's rules already cover it: deleting the profile keeps it, and "Delete all data" removes it.
`docs/01_architecture/04_database.md`'s `Prompt` entity becomes this setting rather than a table.

### 6. Guidance is validated in core

Saving guidance is refused, with a message that says why, when it is:

- for a key that is not one of the three
- empty once trimmed — resetting is how a prompt goes back to its default, and an empty instruction is not a choice to keep
- longer than 2,000 characters, so it cannot crowd a small local model's context out from under the material it writes from

The text is otherwise the user's, and is not scanned for what it asks.

### 7. A default that changed under a user's guidance is pointed out

When a release changes a default, a user who replaced it keeps their guidance.
Where the guidance is shown, a stored `default_digest` that differs from the current default's says "The default has changed since you edited this", with the new default beside theirs, so an improvement is not silently missed.
Saving again records the current digest.

### 8. Where it is changed

- **HTTP:** `GET /v1/settings/prompts` lists the three, each with its key, a title, what it is used for, the default, the user's guidance if any, and whether the default has changed since they saved.
  `PUT /v1/settings/prompts/{key}` saves guidance; `DELETE /v1/settings/prompts/{key}` resets it.
- **CLI:** `jobpilot-cli settings prompts` lists them; `settings prompts show <key>`, `settings prompts edit <key>` (in `$EDITOR`, starting from the current guidance) and `settings prompts reset <key>`.
- **Desktop:** Settings → Prompts, one card per prompt: what it is used for, the guidance in an editor, "Reset to default", and the change notice (decision 7).
  The cover letter's card says that length and tone belong in About you's writing preferences, and links there.

A saved change applies from the next request, as model settings do; nothing is rescored or rewritten.
A job already scored keeps its score until it is analyzed again.

## Consequences

- PR-013 is met, and `docs/02_features/13_settings.md` loses "prompt customization" from its unimplemented categories.
- The three prompts are restructured into frame and guidance in core, with tests that the frame is present whatever the guidance is, that the guidance sits before the rules, and that the reminder's style line follows the default only.
- The matching prompt gains a grounding rule (decision 2), which is a change to every analysis, customized or not, and is measured on the existing fixtures before it ships, alongside the new default guidance.
- The defaults are documented once, in a new feature spec, `docs/02_features/20_prompt_customization.md`, with its acceptance criteria; the code holds the text and the spec quotes it.
- Scores written under different guidance sit side by side in the Jobs list until rescored. Nothing records which guidance produced a score; if that proves confusing, recording it is a later, separate change.
- The four extraction prompts stay fixed. Offering one later needs its own case, made against what its validation can catch.

## What building it changed

- **A changed default can be kept without editing.** Decision 7 cleared the notice by saving again, and Save is offered only for a change, so a user content with their guidance had no way to say so. The notice has "Keep yours", which saves the guidance as it is and records the default they have now seen.
- **The list says how long guidance may be.** `GET /v1/settings/prompts` carries `max_length`, so the desktop counts against core's number rather than a copy of it. It counts by character, as core does, so an accented letter is one.
- **The CLI can edit without an editor.** `settings prompts edit` takes `--file`, or `-` for standard input, as `profile about set` already did, so guidance can be scripted.
- **The written answer's "why this job" rule is frame.** Answering it from the candidate's own answer is about which material is used (`docs/02_features/16_about_you.md` D5), not about style, so it was not lifted into the default.
- **The parts of a system prompt are labelled in plain words.** Decision 3 placed the guidance "under a heading of its own"; as `### Guidance` and `### Rules`, two letters in four on `llama3.2:1b` copied the posting's own `###` sections back. Labelled "Guidance:" and "These rules take precedence over the guidance above:", none in eight did.
- **The reminder gained nothing.** A line asking for the letter alone, with nothing before it, made all eight letters open "Here is a tailored cover letter". Under the user's own guidance, where the reminder drops the default's style line as decision 3 says, three letters in four open that way; the candidate removes it in review, and catching it belongs to the draft check.
- **The matching change was measured**, as Consequences required, on `llama3.2:1b` in two rounds: every reply stayed valid, and the skills the model listed as matched that the candidate did not have fell from 34 to 20 and from 27 to 20, nearly all on postings the candidate does not fit. Scores on a model that size do not separate a fit from a miss before or after. The tables are in `docs/02_features/20_prompt_customization.md`.

Measured in the desktop and the CLI against a scratch daemon: guidance saved from either is read back by the other, an empty or over-long one is refused, a default changed under the user's guidance is pointed out and can be kept or reset, and a reset prompt shows its default.

## Alternatives considered

**The whole system prompt editable.**
The literal reading of PR-013, and the one that lets a user delete the untrusted-content rule, the grounding rule, or the output contract the parser relies on.
The first two are the product's principles, not preferences; the third turns an edit into a feature that stops working.
"Safe boundaries" rules it out.

**Guidance added to the default, never replacing it.**
Safer-looking, and a user who wants their letters to open differently is left arguing with a rule they cannot remove.
The default style is taste; taste is what the user is being offered.

**All seven prompts.**
The four extraction prompts have no taste to change, and the apply control is where a wrong instruction sends a browser somewhere.

**The guidance in the user message, where the reminder is.**
It would be read last, which a small model follows best; and it would sit beside the posting, as the kind of instruction the frame tells the model to ignore there.

**A `prompts` table.**
The database document lists a `Prompt` entity, and three rows of user text with no relations are a setting.
A table would add a record kind to ADR-0021 for nothing it does not already do with settings.

**Length as guidance.**
The model would write to the guidance's length and the draft check would flag it at the preferences', telling the user their own letter was too long.
