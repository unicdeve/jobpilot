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

The DSN is the right place for these because `database/sql` maintains a
connection *pool*: a pragma executed once via `db.Exec` lands on whichever
connection that call happened to take, leaving the rest of the pool
unconfigured. Both failure modes this caused were real — concurrent API
requests surfacing "database is locked" in the desktop app, and foreign key
constraints going unenforced on most connections.

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
