# Current Development State

## Phase

Roadmap Phases 0–6 (`docs/00_product/roadmap.md`) are substantially
implemented — foundation, candidate profile, job discovery, matching,
application generation, a desktop app, and a browser automation worker —
with explicit gaps noted below. Phase 7 (review mode's "browser handoff"/
submission tracking) and Phase 8 (autopilot policy engine) have not started.

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
- Candidate profile management: get, update, delete. Name is stored as
  separate `first_name`/`last_name` columns (migration
  `00004_candidate_name_split.sql`), not one combined field — this fixed a
  real bug found live during Phase 6 verification, where `profileFactAnswer`
  matched any question label containing "name" and put the candidate's
  full name into both a form's "first name" and "last name" fields;
  `Candidate.FullName()` is used wherever a single display string is
  wanted (cover letters, CLI summaries).
- Resume management: import (via Go MarkItDown), list, delete, with an
  optional review-gated LLM suggestion step (first/last name, contact,
  location, Summary, Skills draft — `ResumeService.ProposeProfileDraft`)
  applied to the candidate profile after import. Two entry points share
  this same mechanism: the CLI onboarding flow (prompt-per-field with the
  suggestion as the default, applied via an explicit `UpdateProfile` call)
  and the desktop Profile screen's "Import resume to prefill" button (fills
  the in-progress form; nothing reaches the daemon until the user hits
  Save/Create profile, via an "Overwrite existing fields with resume data"
  toggle, default off, that only fills currently-empty fields unless
  checked). A draft field the LLM didn't find in the resume is left blank
  rather than guessed.
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
- Custom job URL import (`docs/02_features/04_job_discovery.md` AC-007):
  neither Arbeitnow nor Remotive's public API can fetch an arbitrary job by
  URL, so `JobDiscoveryService.ImportJob` falls back to
  `importViaBrowser` — opening the URL in the same Playwright worker Phase 6
  built for application prep, reading the rendered page (handles
  JS-rendered job boards a plain HTTP fetch couldn't), and asking the LLM
  to extract structured fields (title, company, location, remote/employment
  type, salary if stated, skills, and a cleaned job description) from the
  page's converted markdown — the same "extract, review-gated, never
  invent" pattern `ResumeService.ProposeProfileDraft` already established.
  Unlike `PrepareApplication`, the browser window is closed once the page
  is read rather than left open, since importing a job isn't a submission
  flow. A detected CAPTCHA/blocker, or a page the LLM can't find a job
  title on, fails clearly rather than saving a garbage job record.
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
  Activity nav screen (see below — a per-application activity panel exists
  instead) and any settings category beyond LLM provider/model (same
  reason — unimplemented backend).

- Browser automation (Phase 6, `internal/adapters/secondary/browser`,
  `docs/01_architecture/03_browser_worker.md`): a real Go Playwright
  (`github.com/mxschmitt/playwright-go`) worker that opens the user's actual
  installed Chrome — never Playwright's bundled browser — in a dedicated,
  JobPilot-owned profile (never the user's daily-driver Chrome profile, so
  it can't conflict with it and never needs their real passwords; the user
  logs into any job board once inside this profile). `application prepare-
  in-browser <id>` / the desktop's "Prepare in browser" button navigates to
  the job posting, extracts the form (label association follows the same
  chain browsers use: `<label for>` → wrapping `<label>` → `aria-label` →
  `placeholder` → name, with radio buttons sharing a name grouped into one
  question), turns each field into an `ApplicationQuestion` via
  `AnswerService.AddExtractedQuestion` (unchanged resolver chain reused as-
  is), and fills in whatever resolved — leaving `needs_user_input` fields
  blank in the still-open browser, exactly like a manually entered
  unresolved question today. On success the application moves
  `PREPARING → READY_FOR_REVIEW`; on a CAPTCHA, an unreadable form, or a
  fill failure it moves to `FAILED` with the reason. **It never clicks the
  target site's own submit button** — the browser window is always left
  open (on every path, including failure) for the human to review and
  submit themselves, which is what actually satisfies Review Mode's
  explicit-approval requirement here; see the "Submission boundary" section
  added to `03_browser_worker.md`. `AgentRun`/`AgentEvent`
  (`internal/core/domain/agent_run.go`) get their first real implementation
  here (schema existed since migration 00001, unused until now), each run
  scoped to one application (migration `00003_agent_runs_application_link.sql`,
  which also adds `source`/`field_ref` to `application_questions`) and
  surfaced via `GET /v1/applications/{id}/agent-runs` and a panel on the
  desktop's Applications detail screen.

## Known gaps within Phases 0–6

- No settings/configuration system beyond LLM provider/model
  (`docs/02_features/13_settings.md` — other categories like prompt
  customization, search preferences, and notifications are unimplemented).
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
- `AnswerService`'s deterministic-derivation step is intentionally a single
  rule (remote-work preference) rather than a general capability — e.g.
  computed years of experience isn't derivable because `Candidate.Experiences`
  is an unstructured text blob, not structured dates (the same normalization
  gap noted above).
- No policy engine (Phase 8) integration — state transitions, including
  ones the browser worker makes, are user/CLI/desktop-driven, not
  autopilot-evaluated; `ApplicationMode` "autopilot" exists as a value but
  nothing branches on it yet.
- The browser worker's CAPTCHA/anti-bot detection
  (`internal/adapters/secondary/browser/playwright_worker.go`) is a
  best-effort heuristic (known CAPTCHA iframe hosts, common "verify you're
  human" phrases), not a real detector — it can miss novel challenges (which
  then likely surfaces as an extraction/fill failure instead, the same safe
  stop) or rarely false-positive on ordinary pages containing similar text.
- Radio button groups are extracted as one question with the group's
  `<fieldset><legend>` text as its label, falling back to a humanized
  `name` attribute when there's no fieldset/legend — real-world forms
  without either markup pattern will show a rough, not fully human-authored
  label.
- Salary fields on extracted forms aren't filled: `ApplicationQuestion` has
  no structured salary type, so a form's salary field becomes an ordinary
  text question resolved (or not) like any other, rather than something
  purpose-built for numeric/range salary inputs.
- The browser worker requires the user to log into any job board once
  inside its dedicated, JobPilot-owned Chrome profile (never their real
  daily-driver profile) before "prepare in browser" can get past an
  authentication wall on that board — there is no guided first-use flow
  for this yet.

## Next

1. Phase 7 (review mode's "browser handoff" polish, submission tracking)
   and Phase 8 (autopilot policy engine) — the natural next steps now that
   Phase 6 gives the browser worker something to hand off from. Remaining
   Settings categories (prompt customization, search preferences,
   notifications, data retention) are also still open.
2. The undocumented gaps above (entity normalization, structured LLM
   output, richer answer derivation) once their preconditions land, plus
   the browser-worker-specific gaps just noted (CAPTCHA heuristic, radio
   grouping fallback, salary fields).
