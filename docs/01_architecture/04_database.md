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
