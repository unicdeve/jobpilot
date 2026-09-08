# Current Development State

## Phase

Roadmap Phases 0–5 (`docs/00_product/roadmap.md`) are substantially
implemented — foundation, candidate profile, job discovery, matching,
application generation, and a desktop app — with explicit gaps noted below.
Phase 6 (browser automation) has not started.

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

### Features implemented (CLI unless noted; see gaps below)

- Onboarding flow: data directory + schema init, LLM connectivity check,
  initial candidate profile, optional resume import.
- Candidate profile management: get, update, delete.
- Resume management: import (via Go MarkItDown), list, delete, with an
  optional review-gated LLM suggestion step (Summary/Skills draft) applied
  to the candidate profile after import.
- Job discovery: search and list against two real, no-auth public job-board
  APIs (`internal/adapters/secondary/discovery/arbeitnow.go`, `remotive.go`),
  with canonical-URL deduplication. Each connector caches its fetched
  listing in memory (15 min for Arbeitnow, 4 hours for Remotive) matching
  that API's own stated update cadence, so repeated searches don't poll
  faster than their data actually changes; job descriptions (HTML) are
  converted via the same MarkItDown engine resume import uses. Keyword
  filtering happens client-side (neither API's own search parameter proved
  reliable) via the same naive substring match the old mock used — precise
  for specific terms ("golang") but noisy for short/common ones ("go"),
  a known limitation carried over rather than newly introduced.
  `MockJobSource` remains in the codebase for tests/dev but is no longer in
  the daemon's production source list, so real searches are never mixed
  with fake jobs.
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
  revised the Phase-0-era `applications`/`application_questions` tables —
  versioned `cover_letters` (replacing a single mutable column, so
  regeneration keeps history), a durable `application_state_transitions`
  table, and an answer `category` column matching the four-category
  resolver in `docs/02_features/07_application_answers.md`.
- Application generation (Phase 4, `jobpilot-cli application ...`):
  `ApplicationService` enforces a validated state machine
  (`docs/02_features/08_applications.md`) as an explicit Go rule table, with
  `SUBMITTED` reachable only from `APPROVED` through one explicit call.
  `CoverLetterService` generates cover letters grounded in the candidate
  profile and (if chosen) resume, with the job description wrapped as
  untrusted content the same way job matching already does; each
  generate/regenerate call creates a new version rather than overwriting the
  previous one, and versions are user-editable. `AnswerService` resolves
  application questions through an explicit four-step chain (exact profile
  fact → one deterministic derivation rule → LLM generation for narrative
  questions only → left for the user) — an unresolved factual question
  structurally never reaches the LLM, per
  `docs/02_features/07_application_answers.md` AC-003.

- Desktop app (Phase 5, `desktop/`): Tauri 2 + React + TypeScript + Tailwind
  v4, per `docs/01_architecture/02_desktop.md`. A Rust `ensure_daemon`
  command (`desktop/src-tauri/src/daemon.rs`) locates or spawns
  `cmd/jobpilot` using the exact same discovery-file protocol as
  `cmd/jobpilot-cli/daemon.go`, so the CLI and desktop app agree on the
  running daemon; all other calls go from TypeScript straight to the
  daemon's loopback API via a typed client
  (`desktop/src/lib/api.ts`) mirroring `internal/adapters/primary/httpapi/client.go`.
  Screens: Dashboard, Profile, Resumes, Jobs, Matches, Applications (the
  review/approval UI — state transitions, cover letter versions,
  question resolution), Settings. Accessible components are generated via
  the shadcn/ui CLI (Button, Card, Badge, Input, Textarea, Label, Select,
  Checkbox), per `02_desktop.md`; a small `StatusBadge`/`FormField` wrapper
  in `desktop/src/components/` layers the app's per-state colors and
  label+control pairing on top of those primitives. Not built: Agent
  Activity (no service populates `AgentRun`/`AgentEvent` yet) and any
  settings category beyond LLM provider/model (same reason — unimplemented
  backend).

## Known gaps within Phases 0–5

- No settings/configuration system beyond LLM provider/model
  (`docs/02_features/13_settings.md` — other categories like prompt
  customization, search preferences, and notifications are unimplemented).
- Custom job URL import (`docs/02_features/04_job_discovery.md` AC-007) is
  currently unsupported: neither Arbeitnow nor Remotive's public API can
  fetch an arbitrary job by URL (only search/listing), so
  `JobDiscoveryService.ImportJob` returns a clear "no available source
  supports importing jobs by URL" error instead of the old mock's
  fake-success behavior for two hardcoded demo URLs. Real support needs
  either a connector whose API does support single-job lookup, or the
  generic page-scraping Phase 6 (browser automation) is expected to add.
- No persisted search-history feature (Phase 2).
- No user-configurable prompt customization for matching (Phase 3).
- The Desktop app has no onboarding wizard equivalent to the CLI's
  `onboard` command (LLM connectivity check, guided first-run flow) —
  the Profile screen's "create profile" form covers only the candidate
  profile step. Also no Agent Activity screen (see above) and no import
  progress/loading polish beyond basic "Loading…" text.
- Some core entities from `docs/01_architecture/04_database.md`
  (`CandidateExperience`/`CandidateEducation`/`CandidateSkill`, `JobSource`,
  `Prompt`, `Notification`) are not separate tables — candidate
  skills/experience/education are JSON-blob columns on `candidates`, jobs
  reference their source only by free-text ID, and there is no schema home
  yet for prompt customization or notifications. This deviation from the
  architecture doc is undocumented in an ADR and unaddressed so far.
- The LLM provider abstraction (`internal/core/ports/llm.go`) has no
  schema-constrained structured output support: matching, profile-draft, and
  cover-letter/answer generation all rely on prompt instructions plus
  post-hoc parsing, not provider-level schema constraints, despite
  `docs/01_architecture/05_llm.md` preferring the latter where supported.
- Application questions have no source but manual CLI entry
  (`application question add`): there is no form-extraction yet (Phase 6,
  browser automation, is post-MVP), so `docs/02_features/07_application_answers.md`
  can only be exercised against hand-entered questions for now.
- `AnswerService`'s deterministic-derivation step is intentionally a single
  rule (remote-work preference) rather than a general capability — e.g.
  computed years of experience isn't derivable because `Candidate.Experiences`
  is an unstructured text blob, not structured dates (the same normalization
  gap noted above).
- No browser automation (Phase 6) or policy engine (Phase 8) integration for
  applications yet — state transitions are user/CLI-driven, not
  autopilot-evaluated.

## Next

1. Remaining Settings categories (prompt customization, search preferences,
   notifications, data retention), and browser automation (Phase 6) — which
   would also unblock real custom-URL job import (see gap above) and
   application question sourcing.
2. The undocumented gaps above (entity normalization, structured LLM
   output) and the application-generation gaps just noted (question
   sourcing, richer derivation) once their preconditions land.
