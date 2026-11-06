# ADR-0022: What the candidate says about themselves

## Status

Accepted, and implemented.
Narrowed by ADR-0034 (`0034-the-candidate-writes-and-fills-their-own-application.md`) on 2026-11-06: nothing is generated from these answers any more, and the writing sample, writing preferences, "never mention" question and per-application answers are removed.
Standing answers and kept answers stand, as the candidate's own copy-ready material.
It records `docs/02_features/16_about_you.md` D1 (decided on 2026-09-29), and how that decision is built.

## Context

`docs/02_features/16_about_you.md` asks the candidate for material only they have: what they want next, what they value, their stories, how they sound.
It comes in four forms:

- **Standing answers**, to a fixed set of questions, answered once for every application.
- **A writing sample**, used for voice only.
- **Per-application answers**, to a fixed question and up to two the model proposes from the posting.
- **Kept answers**: a narrative answer the candidate wrote on a form, kept to inform later ones.

There is also a fifth, which exists already: `candidates.application_preferences`, one free-text field for tone and length, which the spec moves in beside the standing answers.

ADR-0015 normalized the candidate facts a form *looks up* — email, phone, each kind of link, work authorization per country — because a resolver matching a label against a blob answered the phone field with an email address.
The material here is not looked up.
Nothing fills a field with it directly; it is given to the model as material for prose.
What ADR-0015 was protecting still applies, though: each answer must stay attached to the question it answers, or a prompt cannot tell the candidate's values from their proudest work.

## Decision

### 1. One table of answers, each owned by the candidate or by one application

```sql
CREATE TABLE personal_answers (
    id             TEXT PRIMARY KEY,
    candidate_id   TEXT REFERENCES candidates(id)   ON DELETE CASCADE,
    application_id TEXT REFERENCES applications(id) ON DELETE CASCADE,
    key            TEXT NOT NULL,
    question       TEXT NOT NULL DEFAULT '',
    answer         TEXT NOT NULL DEFAULT '',
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,
    CHECK ((candidate_id IS NULL) != (application_id IS NULL))
);
```

Exactly one owner, and the owner is what a row goes with: a candidate's rows when the profile is deleted, an application's when it is.
Those are the two rules ADR-0021's table gains, for a new record kind, `personal_answer`, and its agreement test holds the schema to them.

### 2. A row's key says what it is

| Key | Owner | Question | Rows |
| --- | --- | --- | --- |
| `looking_for`, `values`, `proudest_work`, `known_for`, `career_story`, `never_mention` | candidate | the wording it was answered under | one per key |
| `writing_sample` | candidate | empty | one |
| `writing_preferences` | candidate | empty | one |
| `kept` | candidate | the form's label | any number |
| `draws_you` | application | the wording it was answered under | one |
| `proposed` | application | the model's question | at most two |

The keys and the wording of each question are core's, in `internal/core/domain`, and clients render the questions the daemon returns rather than holding a copy.
A prompt refers to an answer by its key, so a question's wording can improve without changing what its answer means.
The one-per-key rules are a partial unique index; the cap of two proposed questions is core's, which is the only writer.

An answer is prose, and a cleared one is a deleted row: an empty answer is not an answer.
A proposed question is the exception, stored before it is answered, so rewriting a letter offers the same questions rather than new ones.

### 3. Writing preferences move into the table

Migration `00018` copies each non-empty `candidates.application_preferences` into a `writing_preferences` row and drops the column.
The profile no longer carries the field; it is read and written with the rest of what the candidate says about how they sound.

This keeps one place for "how to write about me", where the spec puts it on screen.
Left as a column, the profile form and the About you form would both write guidance about the letter, from two screens, through two endpoints.

### 4. Only the candidate writes an answer

No path writes a model's output into `answer`.
The model's only contribution to the table is a proposed question, and that is held to:

- at most two per application, proposed once
- one line, at most 160 characters, ending in a question mark; anything else is dropped rather than repaired
- shown as plain text
- included in a later prompt only inside an untrusted-content block, since it was derived from the posting

The posting is untrusted input (AGENTS.md principle 8), but a proposed question reaches no employer and triggers no action: the candidate reads it and chooses whether to answer.

### 5. One place assembles the material

A single function in core renders what the candidate has said into a prompt, for both the cover letter and a narrative answer, so the two cannot drift apart:

- the standing answers, by key, and this application's answers
- `never_mention`, as a constraint rather than material
- the writing sample, delimited and labelled as style only, with an instruction that nothing in it may be stated as a fact
- `writing_preferences`, as the tone and length to follow
- for a narrative answer, up to three kept answers, chosen by how many of the question's words their question shares, newest first on a tie

Choosing the kept answers is deterministic and in core; the model is never asked which are relevant.

## Consequences

- One new table, and one table change that drops a column after copying it.
  The copy is exact: what `application_preferences` held is what `writing_preferences` holds.
- The profile's JSON loses `application_preferences`.
  The CLI and the desktop are the only clients, and change with it.
- `personal_answer` joins ADR-0021's record kinds, with a name in the CLI's and the desktop's deletion wording.
- A question's wording lives in one place, so the CLI and the desktop cannot ask it differently.

## Alternatives considered

**A column per standing question.**
Readable, but every new question is a migration, and per-application and kept answers need tables of their own anyway — three shapes for one idea.

**One free-text "about me" field.**
The shape ADR-0015 spent a change removing: nothing can tell which part of it answers which question, so a prompt cannot use `never_mention` as a constraint or the proudest work as a story.

**A table per kind.**
Four tables with the same five columns, and four sets of deletion rules, to avoid one `CHECK` constraint.
