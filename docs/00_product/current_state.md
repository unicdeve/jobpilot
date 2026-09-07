# Current Development State

## Phase

Roadmap Phases 0–3 (`docs/00_product/roadmap.md`) are substantially
implemented — foundation, candidate profile, job discovery, and matching —
with explicit gaps noted below. Phase 4 (application generation) has not
started.

## Implemented

### Decisions agreed

- Product concept agreed
- Local-first architecture agreed
- Go as primary runtime agreed
- CLI + desktop agreed
- SQLite agreed
- Ollama as initial local LLM agreed
- Tauri + React desktop direction agreed
- Go Playwright direction agreed
- Go MarkItDown direction agreed
- Assist/Review/Autopilot modes agreed
- Review is default
- Cross-platform target agreed

### Features implemented (CLI-only; see gaps below)

- Onboarding flow: data directory + schema init, LLM connectivity check,
  initial candidate profile, optional resume import.
- Candidate profile management: get, update, delete.
- Resume management: import (via Go MarkItDown), list, delete, with an
  optional review-gated LLM suggestion step (Summary/Skills draft) applied
  to the candidate profile after import.
- Job discovery: search, list, import-by-URL, with canonical-URL
  deduplication — currently against a deterministic `MockJobSource` only.
- Job matching via Ollama: structured JSON output (score, confidence,
  strengths, gaps, missing skills, rationale), persisted per candidate/job.
- SQLite persistence for candidates, resumes, jobs, and matches, with
  goose-based versioned migrations (embedded in the binary).
- LLM provider/model settings (`jobpilot-cli settings show`/`set-llm`):
  choose from known local providers (Ollama, llama.cpp, LM Studio) or a
  custom OpenAI-compatible endpoint, with a manually-entered model name;
  persisted in SQLite's `user_settings` table. A single active provider
  configuration is supported, per `docs/01_architecture/05_llm.md`.
- Automated test suite (Phase 0 "Test infrastructure"): unit tests for
  `internal/core/services` (fakes, no I/O) and pure-logic helpers, plus
  integration tests (`-tags=integration`) for the SQLite repositories
  (against a real temp-file database) and the LLM provider adapters
  (against a fake HTTP server), per `docs/08_development/testing.md`.
- Daemon/IPC boundary (`docs/09_decisions/0012-loopback-http-ipc.md`):
  `cmd/jobpilot` is a real process that owns the database and every core
  service, exposed as JSON over loopback HTTP
  (`internal/adapters/primary/httpapi`). `cmd/jobpilot-cli` no longer opens
  SQLite or wires services directly — it is a thin client that auto-starts
  the daemon (via a discovery file in the data directory) if none is
  reachable. This closes the gap between the CLI-direct wiring that existed
  through Phase 3 and the client/daemon architecture described in
  `docs/01_architecture/00_overview.md` and `07_ipc.md`.
- Application schema (`internal/adapters/secondary/sqlite/migrations/00002_application_schema.sql`):
  revises the Phase-0-era `applications`/`application_questions` tables
  ahead of Phase 4 feature work — versioned `cover_letters` (replacing a
  single mutable column, so regeneration keeps history), a durable
  `application_state_transitions` table, and an answer `category` column
  matching the four-category resolver in
  `docs/02_features/07_application_answers.md`. No domain/service/repository
  code uses these tables yet; that is the actual Phase 4 feature work.

## Known gaps within Phases 0–3

- No settings/configuration system beyond LLM provider/model
  (`docs/02_features/13_settings.md` — other categories like prompt
  customization, search preferences, and notifications are unimplemented).
- Job discovery has no real public connectors yet, only the mock source used
  for development.
- No persisted search-history feature (Phase 2).
- No user-configurable prompt customization for matching (Phase 3).
- The Desktop app (Phase 5) hasn't started; the daemon/IPC layer it depends
  on is now implemented (see above), but only the CLI uses it so far.
- Some core entities from `docs/01_architecture/04_database.md`
  (`CandidateExperience`/`CandidateEducation`/`CandidateSkill`, `JobSource`,
  `Prompt`, `Notification`) are not separate tables — candidate
  skills/experience/education are JSON-blob columns on `candidates`, jobs
  reference their source only by free-text ID, and there is no schema home
  yet for prompt customization or notifications. This deviation from the
  architecture doc is undocumented in an ADR and unaddressed so far.
- The LLM provider abstraction (`internal/core/ports/llm.go`) has no
  schema-constrained structured output support: matching and profile-draft
  generation rely on prompt instructions plus post-hoc JSON parsing, not
  provider-level schema constraints, despite `docs/01_architecture/05_llm.md`
  preferring the latter where supported.

## Next

1. Begin Phase 4 (application generation): application domain
   types/repository/service, cover letter generation, application answers —
   building on the revised schema above.
2. Longer-term: real job-board connectors (Phase 2 completion), remaining
   Settings categories (prompt customization, search preferences,
   notifications, data retention), Desktop app (Phase 5), and the two
   undocumented gaps above (entity normalization, structured LLM output).
