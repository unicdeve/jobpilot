# Feature: Data Deletion and "Not Interested"

## Status

Implemented.
D1 to D10 were decided on 2026-09-25, and ADR-0021 (D1) records how deletion is built.
Migration `00015` gives the foreign keys their rules and `00016` adds `jobs.not_interested_at`.
ADR-0032 scopes “delete all data” to the selected local profile once more than one profile can exist.

## Purpose

Let the user delete what JobPilot has stored about them and their search, and have deletion take with it everything derived from what was deleted.

This is what `docs/00_product/acceptance_criteria.md` AC-G-021 and AC-G-022 require, and what `docs/07_security/02_data_retention.md` lists: individual jobs, applications, generated documents, resumes, history, and all application data.
All of it is local, so deletion is the user's main control over their own data.

Deleting a job and not wanting a job are different requests, so this spec also covers "Not interested" (D2): a job the user has ruled out stays stored, leaves their lists, and is skipped by discovery and scouts.

## What exists today

| What | Deletable | Where |
| --- | --- | --- |
| Resume | yes, with a defect (below) | CLI, desktop, `DELETE /v1/resumes/{id}` |
| Saved search | yes; its scouts go with it (`ON DELETE CASCADE`) | CLI, desktop |
| Scout | yes; its runs stay, unlinked (`ON DELETE SET NULL`) | CLI, desktop |
| API key | yes | CLI, desktop |
| Profile | `ProfileService.DeleteProfile` exists; nothing calls it | — |
| Job, match, application, cover letter | no | — |
| History, all data | no | — |

Two defects follow from the schema, not from any decision.

Deleting a resume that an application used fails.
`applications.resume_id` references `resumes` with no `ON DELETE` rule, and foreign keys are enforced, so the daemon answers 500 with `FOREIGN KEY constraint failed`.
Measured against a scratch installation; the desktop's Resumes screen hits it too.
`docs/00_product/05_edge_cases.md` lists this exact case.

`docs/02_features/12_agent_activity.md` says "deleting the scout or the application keeps the run and clears the link".
That holds for a scout.
For an application it cannot: `agent_runs.application_id` has no `ON DELETE` rule either, so the database would refuse to delete an application that has ever been prepared in the browser.

### What references what

| Table | References | On delete |
| --- | --- | --- |
| `resumes` | `candidates` | none |
| `job_matches` | `jobs`, `candidates` | none |
| `applications` | `jobs`, `candidates`, `resumes`, `cover_letters` (current version) | none |
| `application_questions` | `applications` | none |
| `cover_letters` | `applications` | none |
| `application_state_transitions` | `applications` | none |
| `agent_runs` | `applications` | none |
| `agent_runs` | `scouts` | set null |
| `agent_events` | `agent_runs` | none |
| `candidate_experiences`, `candidate_skills`, `candidate_links`, `candidate_work_authorizations` | `candidates` | cascade |
| `scouts` | `saved_searches` | cascade |

`applications` and `cover_letters` reference each other, so an application and its letters have to be deleted together, clearing `current_cover_letter_id` first.

SQLite cannot change a foreign key's `ON DELETE` rule in place.
Changing one rebuilds the table: create it anew, copy the rows, drop the old one, rename.

## What is stored, and where

In the selected workspace's database (`jobpilot.db` for the migrated first profile or `workspaces/<id>/jobpilot.db` for a later one):

- about the user: the candidate and its four child tables, resumes (as converted text; the original file is never copied, only its path is recorded)
- derived from the user and a job: matches, applications with their questions and answers, cover letter versions and state transitions
- about the search: jobs, saved searches, scouts, watched companies (ADR-0024)
- the trail: agent runs and their events
- configuration: LLM settings, the names of stored API keys (never the keys)

Outside it:

- API keys, in the OS keychain (ADR-0019)
- the workspace's browser worker Chrome profile, `browser-profile/`, which holds whatever job boards that profile signed into
- the installation-global `daemon.log`, `daemon.json` and `workspaces.json` in the data directory
- the desktop's theme choice, in the webview's own storage

Deleted rows are not gone from disk when a `DELETE` returns.
SQLite leaves their content in free pages and in the WAL file until those are overwritten, checkpointed or vacuumed.

What an answer or a trail step contains matters for what "derived" means.
An application's questions hold the answers given to the employer, which are candidate facts.
A browser run's steps name the questions a form asked and the pages visited; since migration 00012 they never hold the answer typed.
A scout run's steps name the jobs it passed over or applied for, by title.

## Principles

1. Deletion is explicit.
   Nothing deletes on a timer or as a side effect of something else the user did.
2. The user is told what goes before it goes.
   A confirmation names what will be deleted with the thing chosen, by kind and count, and says it cannot be undone.
3. What was derived from deleted data goes with it (AC-G-022).
   The database must not be able to keep an orphan, and must not refuse a deletion the product allows.
4. What goes is decided by deterministic rules in core, never by a model, and tested there.
5. Deletion contacts nothing outside the machine.
   It never withdraws an application with an employer or signs out of a job board; it says so where that could be expected.
6. There is no undo.
   The confirmation is the safety.

## Behaviour

Each operation deletes the thing chosen and everything derived only from it, in one transaction.

### Delete a job

Deletes the job, its match, and its applications with everything under them (see "Delete an application").
When the job has applications, the confirmation names them and their state: "This job has 2 applications, one of them submitted."

Discovery stores the job again the next time a search finds it, and a scout may then create an application for it.
The confirmation says so and offers "Not interested" instead.
Deleting a job marked not interested forgets that decision with it.

### Delete an application

Deletes the application, its questions and answers, its cover letter versions and its state history.
Its browser runs go with it; see D4.
A submitted application can be deleted; see D3.
The confirmation says that deleting it does not withdraw anything sent to an employer.

A scout may create it again on its next run while the job is stored, unless the job is marked not interested.

### Delete a cover letter version

Deletes one version.
The current version cannot be deleted while it is current: the user picks another first, or deletes the application.
This keeps "the letter this application uses" from silently changing.

### Delete a resume

Deletes the resume.
Applications that used it keep everything already generated from it and record that their resume was deleted (`resume_id` becomes null); see D5.

### Delete the profile

Deletes the candidate and all of its details, its resumes, its matches and its applications with everything under them.
Keeps what is about the search rather than the person: jobs, saved searches, scouts, watched companies, settings and API keys.
The installation then returns to first-run setup.

### Clear history

Two separate actions; see D6:

- **Clear activity** deletes every agent run and its steps.
- **Clear jobs** deletes every stored job that no application uses, with its match.
  Jobs marked not interested are kept, since each holds a decision the user made; see D10.

### Delete all data

Returns the selected local profile to its setup state; see D7 and ADR-0032.
Deletes every row in that workspace's database, every API key JobPilot stored for that workspace, its browser profile directory and its Scout alarms, then compacts the database so the content is gone from disk.
Other profiles, the workspace catalog and the installation-global daemon log stay.
The browser worker's Chrome is closed first, since it holds the profile directory open.
The desktop's theme choice is kept: it is a preference of the device, not data about the user.

The confirmation requires typing a word, not a single click.

## Not interested

"Not interested" rules a job out without deleting it.
It is the user's answer to a job, and it is reversible: "Interested again" undoes it.

### What it does

- **Lists.**
  The job leaves All jobs and Analyzed, the job counts, and the Dashboard.
  A "Not interested" view lists the jobs ruled out, newest decision first, each with "Interested again".
  A job opened directly — from an application, or a link — still shows, with a banner saying it was ruled out and when.
- **Discovery.**
  A search that finds a job marked not interested leaves it out of its result and does not count it as new.
  The job is not stored twice: discovery already recognises it by its canonical URL.
- **Scouts.**
  `Scout.PreScreen` rejects the job before it is scored, so ruling a job out also saves the model call.
  The run records it as a passed-over job with the reason "you marked this job not interested", like any other eligibility decision.
- **Applications.**
  Existing applications for the job are left as they are; see D9.
  The action says how many there are, and the job's page links to them.
- **Matching.**
  The user can still analyze the job from its page; ruling it out does not forbid looking at it.
  Its match leaves the list of analyzed jobs with it, and the job's page reads its own match through `GET /v1/jobs/{id}/match`, which still finds one.

### What it is not

- It is not a deletion: nothing is removed, and the job's description and match stay stored.
- It is not a rule about companies or titles.
  Excluding a company everywhere is the saved search's `ExcludedCompanies`.
- It recognises a job by its canonical URL only.
  The same role cross-posted to a second board under a different URL is a different job, and is not ruled out with the first.

### How it is stored

A nullable `not_interested_at` timestamp on `jobs`, written by the user's action and cleared by "Interested again".
`jobs` references nothing, so this is an added column, not part of the table rebuild D1 requires.
No reason is asked for: nothing would read it, and a field nothing reads is a field that goes stale.

## Decisions

### D1 — Where the rule lives

**Decided: B.**

A deletion's reach is a business rule, so core decides it.

- **A. Schema cascades only.**
  Rebuild the foreign keys with `ON DELETE CASCADE` or `SET NULL` and let SQLite follow them.
  Atomic by construction, but the rule lives in SQL, core cannot test it, and core cannot tell the user in advance what a deletion will reach.
- **B. Core decides, the adapter executes, the schema agrees.**
  A core `DeletionService` computes what a deletion reaches — the counts the confirmation shows — and applies the refusals (the current cover letter).
  A port executes it in one transaction.
  The foreign keys are rebuilt to say the same thing, so the database cannot hold an orphan or reject a valid deletion, and a missed case fails a test rather than leaving data behind.
- **C. Core decides, ordered deletes in the adapter, schema unchanged.**
  No table rebuild, but the foreign keys keep blocking deletes that core has allowed, and the adapter's delete order becomes the only thing standing between the rule and an orphan.

B needs one migration that rebuilds the eight tables above whose rule is "none".
It is a lasting decision about how data leaves the system, so it should be an ADR (ADR-0021) before implementation.

### D2 — Forget, or dismiss

**Decided: C.**
Deleting forgets, and a deleted job can come back.
"Not interested" is specified above and built with deletion.

Deleting a job forgets it, and the next search that finds it stores it again.
Deleting an application lets a scout create it again.
A user who deletes a job may mean "I don't want this data" or "I'm not interested in this job", and those want opposite things.

- **A. Forget only.**
  Deleted means gone; the job can come back.
- **B. Forget, and remember the canonical URL.**
  Discovery and scouts skip a URL the user deleted.
  Keeps one small piece of data about a deleted job, which "delete all data" and "clear jobs" remove.
- **C. Two actions.**
  "Delete" forgets.
  "Not interested" hides the job, keeps it stored, and tells discovery and scouts to skip it.
  Scouts gain a meaningful signal and deletion stays honest, at the cost of a second, small feature.

### D3 — Deleting a submitted application

**Decided: A.**

`docs/00_product/04_application_modes.md` requires "an auditable application status and action history".

- **A. Allow it, with the confirmation saying nothing is withdrawn.**
  The history is audit for the user, and it is the user's to delete.
- **B. Refuse while submitted, interviewing or offered.**

### D4 — The trail of a deleted application

**Decided: A.**

`docs/02_features/12_agent_activity.md` promises a run outlives its application.
AC-G-022 asks for derived data to go with its source, and a browser run's steps name the employer's questions and pages.

- **A. Browser runs go with their application; scout runs stay.**
  A browser run is about one application and says nothing once it is gone.
  A scout run is about the scout and many jobs; it keeps its steps, which name jobs by title.
  The Agent Activity spec changes accordingly.
- **B. Every run stays, unlinked, as that spec says now.**

### D5 — A resume that applications used

**Decided: A.**

- **A. Detach.**
  The applications keep what was generated from it and show that the resume was deleted.
- **B. Refuse, naming the applications.**
- **C. Delete the applications too.**

### D6 — What "clear history" means

**Decided: A.**

- **A. Clear activity and clear jobs, as above.**
- **B. One action that does both.**
- **C. Leave it out, and rely on the individual deletions and "delete all data".**

### D7 — How much "delete all data" deletes

**Decided: A.**

ADR-0032 supersedes the installation-wide scope: option A now applies within the selected local profile, while other profiles and device-global files remain.

- **A. Everything JobPilot stored, including API keys, the browser profile's sign-ins and the log.**
- **B. The database only**, leaving keys, sign-ins and settings for a user who wants to start over without setting up again.

### D8 — Gone from disk

**Decided: A.**

- **A. `PRAGMA secure_delete = ON` on every connection, and a WAL checkpoint after each deletion.**
  Deleted content is overwritten rather than left in free pages, at some cost to write speed that a local, single-user database does not feel.
  "Delete all data" also vacuums.
- **B. Vacuum on "delete all data" only.**
  Individually deleted content may stay on disk until SQLite happens to reuse its pages.

### D9 — Applications for a job marked not interested

**Decided: A.**

- **A. Leave them as they are, and say they exist.**
  An action changes what it names and nothing else, as principle 1 asks of deletion.
  Withdrawing an application is its own act, one click away from the notice.
- **B. Withdraw the ones not yet sent.**
  Moves DISCOVERED, MATCHED, PREPARING, READY_FOR_REVIEW and APPROVED applications to WITHDRAWN.
  Saves clicks, but "withdrawn" has meant "the user withdrew from the employer", and these were never sent.
- **C. Offer B as a choice in the same confirmation.**

### D10 — "Clear jobs" and jobs marked not interested

**Decided: A.**

- **A. Keep them.**
  Each one is a decision; clearing them would let discovery and scouts offer the job again.
  The confirmation says how many are kept, and "delete all data" removes them.
- **B. Delete them with the rest.**

## Where it is offered

- CLI: `job delete <id>`, `application delete <id>`, `application cover-letter delete <id>`, `resume delete <id>`, `profile delete`, `data clear activity`, `data clear jobs`, `data delete-all`.
  Each shows what goes and asks, unless given `--yes`; "delete all data" asks for the word `delete`.
  With no terminal to ask on, nothing is deleted.
  `job not-interested <id>` and `job interested <id>` rule a job out and back in, without confirmation, since both are reversible; `search list --not-interested` lists the jobs ruled out.
- Desktop: Delete in the "⋯" menu of a job's and an application's page, on each earlier cover letter version, and on a resume; profile deletion and the three bulk actions under Settings → Data.
  One dialog serves them all: it fetches the preview as it opens and, for "delete all data", asks for the word `delete`.
  "Not interested" sits in a job page's toolbar and on each row of the job list, shown on hover or focus, and the Jobs screen has a "Not interested" view.
- HTTP: `GET <resource>/deletion` previews and `DELETE <resource>?token=` deletes, for `/v1/jobs/{id}`, `/v1/applications/{id}`, `/v1/cover-letters/{id}`, `/v1/resumes/{id}`, `/v1/profile`, `/v1/data/activity`, `/v1/data/jobs` and `/v1/data`.
  A missing token is 400, a missing record 404, a refusal 409, and a reach that changed 409 with the new reach in the body.
  `PUT /v1/jobs/{id}/interest` sets or clears the mark; `GET /v1/jobs?interest=not_interested|all` includes marked jobs, which it otherwise leaves out.

## Acceptance Criteria

Each is checked by a test: AC-001 to AC-003 by `TestDeletionStore_DeletesExactlyWhatThePreviewSaid` and `TestSchema_ForeignKeysAreTheDeletionRules`, AC-004 by `TestSchema_DeletionFollowsTheRules`, AC-005 by `TestDeletionService_DeleteEverything` and `TestDeletionStore_Specifics`, AC-006 by `TestDeletionStore_DeletedContentIsGoneFromDisk`, and AC-008 to AC-010 by `TestJobDiscoveryService_NotInterested`, `TestScoutRun_PassesOverJobsRuledOut` and `TestJobRepository_NotInterested`.

### AC-001
Each operation above deletes what it names and nothing else, in one transaction.

### AC-002
No deletion leaves a row that references a deleted one, and the database refuses no deletion the product allows.

### AC-003
Before deleting, the user is told what will go with the thing chosen, by kind and count.

### AC-004
A resume that applications used can be deleted (D5).

### AC-005
Deleting all data leaves the installation indistinguishable from a fresh install, apart from the device's theme choice (D7).

### AC-006
Deleted content does not remain readable in the database file (D8).

### AC-007
No deletion contacts an external service.

### AC-008
A job marked not interested is absent from the job lists, the job counts and a search's result, and is listed in the "Not interested" view.

### AC-009
A scout passes over a job marked not interested before scoring it, and records why.

### AC-010
"Interested again" restores the job everywhere it was hidden, and neither action changes the job's applications (D9).

## Not in scope

- Automatic deletion after a retention period, which is the "data retention" settings category and a separate decision.
- Export or backup before deletion.
- The disk-usage cleanup tool that `docs/07_security/02_data_retention.md` mentions as future work.
