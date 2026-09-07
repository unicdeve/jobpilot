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

## Known gaps within Phases 0–3

- No settings/configuration system beyond LLM provider/model
  (`docs/02_features/13_settings.md` — other categories like prompt
  customization, search preferences, and notifications are unimplemented).
- Job discovery has no real public connectors yet, only the mock source used
  for development.
- No persisted search-history feature (Phase 2).
- No user-configurable prompt customization for matching (Phase 3).
- `cmd/jobpilot` (the daemon) is still a stub, and the Desktop app (Phase 5)
  hasn't started — everything above is CLI-only.

## Next

1. Begin Phase 4 (application generation): application schema, cover letter
   generation, application answers.
2. Longer-term: real job-board connectors (Phase 2 completion), remaining
   Settings categories (prompt customization, search preferences,
   notifications, data retention), Desktop app (Phase 5).
