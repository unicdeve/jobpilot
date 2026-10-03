# Database Architecture

## Technology

SQLite.

## Principles

- Local-only by default.
- Transactions for state changes.
- Migrations are versioned.
- Foreign keys enabled.
- Avoid storing secrets.
- Support deletion of user-owned data.
- Use indexes for common queries.
- Use SQLite FTS5 where full-text search is useful.

## Connection configuration

Every connection `database/sql` opens carries the same pragmas, set through
the DSN in `internal/adapters/secondary/sqlite/db.go` rather than by a
one-off `db.Exec` after opening:

| Pragma | Value | Why |
| --- | --- | --- |
| `busy_timeout` | 5000 ms | Without it a connection that finds the database locked fails immediately with `SQLITE_BUSY` instead of waiting. |
| `journal_mode` | `WAL` | In the default rollback-journal mode a writer blocks every reader. WAL matches this application's shape: one writer saving discovered jobs while screens read. |
| `foreign_keys` | `on` | SQLite defaults this off, and it is per-connection — the "Foreign keys enabled" principle above is only true if every pooled connection sets it. |
| `secure_delete` | `on` | A deleted row's content is otherwise left in the file's free pages until SQLite happens to reuse them. Deleting data has to mean it is gone (ADR-0021 §6). Set through `_pragma`, the driver having no shorthand for it. |

The DSN is the right place for these because `database/sql` maintains a
connection *pool*: a pragma executed once via `db.Exec` lands on whichever
connection that call happened to take, leaving the rest of the pool
unconfigured. Both failure modes this caused were real — concurrent API
requests surfacing "database is locked" in the desktop app, and foreign key
constraints going unenforced on most connections.

## Deletion and foreign keys

Every foreign key carries an `ON DELETE` rule, and the rules are `domain.DeletionRules`: `CASCADE` where deleting the referenced record deletes the referencing one, `SET NULL` where it only detaches it (ADR-0021).
`TestSchema_ForeignKeysAreTheDeletionRules` reads every foreign key and compares it with that table, so a new foreign key with no rule, or one that disagrees, fails the build.
A new table must also name its kind of record in `recordTables`.

Deleting a record is deleting its row; the schema takes what the rules say with it.
`DeletionService` works out the same reach from the same rules beforehand, to show the user.
After a deletion the WAL is checkpointed and truncated, and "delete all data" also vacuums, so deleted content does not stay readable in either file.

SQLite cannot change a foreign key in place.
A migration that must is a Go migration registered with `RunDB`, following SQLite's documented rebuild on one held connection: foreign keys off, rebuild in its own transaction, `PRAGMA foreign_key_check`, commit, foreign keys on before the connection returns to the pool.
Migration `00015` is the example to copy, including repairing orphans by their rules before the check.

## Core entities

- Candidate
- CandidateExperience
- CandidateEducation
- CandidateSkill
- Resume
- ResumeDocument
- JobSource
- Job
- JobMatch
- SavedSearch (named search criteria; see `docs/02_features/04_job_discovery.md` AC-001)
- PersonalAnswer (what the candidate says about themselves, owned by the candidate or one application; ADR-0022)
- WatchedCompany (a company whose hiring page discovery reads, with its last check; ADR-0024)
- Application
- ApplicationQuestion
- ApplicationAnswer
- CoverLetter (versioned; an Application references its current one)
- ApplicationStateTransition (durable history of Application state changes)
- AgentRun
- AgentEvent
- Prompt
- Notification (what the user was told about a scout run, or will be, with when it was posted and seen; ADR-0028)
- UserSetting
- Policy

## What a job says about itself

Where a job can be done from, and whether it offers relocation and visa sponsorship, are read from the job's own fields by `domain.ReadSignals` and stored on its row (ADR-0023 §3, migration `00019`):

- `workable_scope` — `anywhere`, `listed` or `unknown`; empty on a row not read yet, which is treated as `unknown`
- `workable_places` — what the locations were read as, in the product's words
- `workable_countries` — the ISO codes they cover, regions expanded, written `,DE,FR,` so one country is a `LIKE '%,DE,%'` away
- `workable_note` — the title or description sentence that made the job `unknown`
- `relocation`, `visa_sponsorship` — `offered`, `mentioned`, `declined` or empty — each with its sentence in `relocation_evidence` and `visa_evidence`
- `signals_version` — the version of the reading that produced them

All of it is derived from columns the row already holds, so no source is asked again to change it.
When the daemon starts, it reads again every job whose `signals_version` is below `domain.SignalsVersion`, in the background, and a change to the vocabulary reaches old jobs as well as new ones.

`saved_searches` gains `work_from`, an ISO code, and `include_relocation`, the two criteria that use them.

The working hours a job asks for are read the same way and stored beside them (ADR-0025, migration `00021`): `hours_status` (`required`, `preferred` or empty), `hours_from` and `hours_to`, UTC offsets without daylight saving, and `hours_evidence`.
`candidates.time_zone` holds the candidate's IANA zone, and `saved_searches` gains `fit_hours` and `hours_shift`.

## Watched companies

`watched_companies` (migration `00020`, ADR-0024 §2) holds each company the candidate watches: its board's provider, region and name, what it is called, the address it was added from and when, and its last check — `checked_at`, `check_status` (`ok`, `gone`, `unreachable` or `failed`) and `open_roles`.
One board is one row, its name compared without case.
It references nothing and nothing references it; "delete all data" deletes it and deleting the profile does not.

Vector search is not required for MVP. Add it only when a concrete retrieval requirement exists.
