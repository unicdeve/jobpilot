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
- Application
- ApplicationQuestion
- ApplicationAnswer
- CoverLetter (versioned; an Application references its current one)
- ApplicationStateTransition (durable history of Application state changes)
- AgentRun
- AgentEvent
- Prompt
- Notification
- UserSetting
- Policy

Vector search is not required for MVP. Add it only when a concrete retrieval requirement exists.
