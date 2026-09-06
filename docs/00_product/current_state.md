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

## Known gaps within Phases 0–3

- No automated test suite anywhere in the repo (Phase 0 "Test
  infrastructure" not started).
- No settings/configuration system yet (`docs/02_features/13_settings.md`
  not implemented) — LLM provider and model are hardcoded defaults, not
  user-configurable.
- Job discovery has no real public connectors yet, only the mock source used
  for development.
- No persisted search-history feature (Phase 2).
- No user-configurable prompt customization for matching (Phase 3).
- `cmd/jobpilot` (the daemon) is still a stub, and the Desktop app (Phase 5)
  hasn't started — everything above is CLI-only.

## Next

1. Add core test coverage (services and repository adapters) to close the
   Phase 0 gap before building further.
2. Implement the Settings feature so LLM provider/model selection is
   user-configurable instead of hardcoded.
3. Begin Phase 4 (application generation): application schema, cover letter
   generation, application answers.
4. Longer-term: real job-board connectors (Phase 2 completion), Desktop app
   (Phase 5).
