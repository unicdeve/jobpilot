# Current Development State

## Phase

Roadmap Phases 0–6 (`docs/00_product/roadmap.md`) are substantially
implemented — foundation, candidate profile, job discovery, matching,
application generation, a desktop app, and a browser automation worker —
with explicit gaps noted below.
Phase 7 (review mode) is complete: application review state and approval UI came with Phases 4 and 5, browser handoff with Phase 6, and submission tracking is described below.
Phase 8 (autopilot policy engine) has not started.

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
  converted via the same MarkItDown engine resume import uses. Filtering
  happens client-side (neither API's own search parameter proved reliable)
  and now lives in `JobDiscoveryService`, not the connectors — see the saved
  searches entry below for the term-matching rule that replaced the naive
  substring match this originally carried over from the mock source.
  `MockJobSource` remains in the codebase for tests/dev but is no longer in
  the daemon's production source list, so real searches are never mixed
  with fake jobs.
- Stored-jobs read path (`GET /v1/jobs`, `JobDiscoveryService.ListJobs`):
  browsing already-discovered jobs is a separate operation from searching
  sources. It is paginated (`limit`/`offset`, default 50, capped at 200),
  optionally filtered by a keyword substring over title and company, and
  returns a `{jobs, total}` envelope so a client can tell whether more pages
  remain. It reads local history only and contacts no source, which is what
  makes AC-006's "existing history remains usable offline" true in practice:
  previously the only way to see a job was `POST /v1/jobs/search`, which
  queries every source over the network and writes what it finds, so both
  `jobpilot-cli search list` (despite its help text) and the desktop Jobs
  screen re-ran a live search just to show a list. Both now read stored
  history instead.

- Saved searches (`docs/02_features/04_job_discovery.md` AC-001,
  `docs/02_features/13_settings.md`): named sets of criteria the user keeps
  and picks between, one of them the default. Stored in `saved_searches`
  (migration `00005`) as explicit columns rather than a JSON blob, managed
  through `jobpilot-cli search saved`, the desktop's Settings → Job searches,
  and an onboarding step that closes the last outstanding requirement in
  `docs/02_features/01_onboarding.md`.

  Discovery now refuses empty criteria. It previously did not, and nothing
  else did either: `SearchCriteria` had eleven fields of which only
  `Keywords` was read by any code, the API accepted the other ten and
  silently ignored them, and an empty keyword short-circuited the connectors
  into returning their whole listing — about 266 jobs across the two boards,
  every one written to the database. The desktop's "Search sources" button
  sent empty criteria whenever the box was blank, so the most obvious action
  in the UI did exactly that. `Title`, `Location`, `RemotePreference`,
  `EmploymentType`, `SalaryMin/Max` and `ExcludedCompanies` are now applied;
  `ExperienceMin/Max` and `IncludedSources` were deleted rather than left
  accepted-and-ignored.

  Filtering moved out of the connectors into `JobDiscoveryService`, behind a
  new `FiltersServerSide` capability
  (`docs/04_connectors/00_connector_sdk.md`). The same filter loop had been
  copy-pasted into all three connectors, sharing a helper defined in the mock
  source, so every new connector had to remember to filter and nothing caught
  it if it did not. Matching is now defined once, in
  `domain.SearchCriteria.Matches`, and matches every term in any order —
  "backend engineer" finds "Engineer, Backend (Go)", which substring matching
  missed. SQLite's stored-job search applies the same term rule, so browsing
  history and searching sources agree about what a query means.

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
  configuration is supported, per `docs/01_architecture/05_llm.md`. The
  daemon resolves those settings per request
  (`internal/adapters/secondary/llm.ReloadingProvider`) rather than once at
  startup, so saving them takes effect immediately: the daemon outlives both
  the desktop app and the CLI, and previously kept using whatever was stored
  when it started until the user found and restarted it.
- Submission tracking (Phase 7, `docs/02_features/08_applications.md`): what happens to an application after it is sent.
  JobPilot hands the user a filled form in a browser and never presses send itself, so only the user knows whether they did — and until now nothing asked.
  An approved application sat approved forever, indistinguishable from one that was sent, and a sent one had nowhere to go but REJECTED.
  INTERVIEWING, OFFERED and NO_RESPONSE record what the employer said, WITHDRAWN became reachable from all three plus SUBMITTED, and no outcome is ever inferred — NO_RESPONSE is the user closing something out, never a timer (AC-007).
  `GET /v1/applications/{id}/transitions` serves the state machine's own answer, so the desktop offers the moves that will work instead of all nine states with an error for most of them, and the CLI's `application show` lists them too.
  The desktop asks the question directly when an application is approved, shows when it was submitted, groups the Applications list by whose move it is, and puts a count of what is waiting on the user on the Dashboard.
  `jobpilot-cli application submitted <id>` is the same act under the name a user would look for.
- Desktop first-run setup (`/welcome`, `docs/02_features/01_onboarding.md`):
  the four things an installation needs — LLM provider, candidate profile,
  optional resume, saved search — in the CLI `onboard` order, since
  prefilling a profile from a resume needs the LLM. A new installation is
  redirected there; skipping is remembered for the session and a banner
  leads back while anything is still missing. Progress is derived from what
  exists rather than from a stored flag, so it resumes at the first gap.
  Previously each prerequisite was reachable on its own screen and each
  screen said what was missing, but nothing put them in an order.
- LLM readiness check (`GET /v1/settings/llm/health`): reports whether the
  configured backend is set up at all and whether it answers right now, so a
  screen can check before starting work that needs it. The desktop's
  "Import resume to prefill" uses it — it previously created the profile and
  imported the resume before failing on the LLM step the user actually
  wanted.
- SQLite connection configuration (`internal/adapters/secondary/sqlite/db.go`,
  `docs/01_architecture/04_database.md`): connection pragmas are set through
  the DSN so they apply to every connection in `database/sql`'s pool, not
  just whichever one a post-open `db.Exec` happened to take. This fixed two
  live defects: concurrent API requests failing outright with
  `database is locked (5) (SQLITE_BUSY)` — visible as an error on the
  desktop dashboard's first load, which disappeared on the next visit
  because by then discovery had nothing new to write — and `PRAGMA
  foreign_keys = ON` leaving most pooled connections with foreign key
  enforcement off, so the schema's `FOREIGN KEY` clauses held only by luck.
  `busy_timeout` makes contention wait rather than fail; `journal_mode=WAL`
  stops a writer from blocking readers.

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

- Desktop server state (`desktop/src/lib/queries.ts`,
  `docs/01_architecture/02_desktop.md`): every screen reads the daemon
  through TanStack Query hooks over the existing typed client rather than a
  per-screen `useEffect`. Request deduplication removes the double fetch
  React StrictMode caused on first render (two concurrent job searches on
  the dashboard, which is what raced on SQLite); the cache means returning
  to a screen no longer refetches; and mutations declare which queries they
  invalidate, replacing the Applications detail screen's habit of reloading
  all five of its endpoints after every action. The Dashboard and Jobs
  screens now read stored jobs via `GET /v1/jobs` instead of running a live
  source search, so opening the app no longer performs a network round trip
  and a database write. The Dashboard's error text no longer appends
  "has a candidate profile been created yet?" to every failure regardless of
  cause; a missing profile is now its own state.

- Desktop jobs list and scroll restoration: the Jobs screen pages through
  stored jobs with `useInfiniteQuery` over `GET /v1/jobs` (50 at a time,
  fetching the next page when a sentinel scrolls into view, with a real
  "Load more" button behind it for keyboard users and for the case where the
  observer never fires). Screen scroll positions survive navigation via
  `desktop/src/lib/use-scroll-restoration.ts` — see
  `docs/01_architecture/02_desktop.md` for why React Router's own
  `ScrollRestoration` does not apply to this layout.

- Job-centric desktop navigation (`docs/01_architecture/02_desktop.md`): a
  job detail route (`/jobs/:id`, served by `GET /v1/jobs/{id}` and
  `jobpilot-cli search show`) shows a stored job with its match, its
  applications, and the actions that create either. Jobs, Matches and
  Applications all link to it, and matches and applications show the job's
  title instead of a bare `job_id`. The forms that asked the user to paste a
  job ID (Matches, Applications) and a resume ID (Applications) are gone:
  analysis starts from the job, an application starts from the job with its
  resume chosen from a list of imported resumes, and creating one opens it.
  This closes the gap between the UI, which had mirrored the CLI's
  verb-per-command shape, and the job-first flows in
  `docs/00_product/03_user_flows.md`.
  The job's description is rendered as markdown
  (`desktop/src/components/markdown.tsx`, react-markdown + remark-gfm) with
  the restrictions the threat model requires for displayed external text:
  no raw HTML, no image loading, http(s) links only and opened in the user's
  browser rather than in the webview.

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
  added to `03_browser_worker.md`. One Chrome, one tab per task (`browser.ChromeSession`): Chrome permits a single instance per profile directory and every task leaves its window open on purpose, so the original worker-per-run launched a second Chrome against a profile the first run had locked and failed for every run after the first.
  Chrome is launched lazily on first use, relaunched if the user closes it, and a task closing its own tab leaves every other task's tab alone.
  `AgentRun`/`AgentEvent`
  (`internal/core/domain/agent_run.go`) get their first real implementation
  here (schema existed since migration 00001, unused until now), each run
  scoped to one application (migration `00003_agent_runs_application_link.sql`,
  which also adds `source`/`field_ref` to `application_questions`) and
  surfaced via `GET /v1/applications/{id}/agent-runs` and a panel on the
  desktop's Applications detail screen.

## Known gaps within Phases 0–6

- No settings/configuration system beyond LLM provider/model and job
  searches (`docs/02_features/13_settings.md` — other categories like prompt
  customization and notifications are unimplemented).
- No user-configurable prompt customization for matching (Phase 3).
- The Desktop app has no Agent Activity screen (see above) and no import
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
- The browser worker never navigates from the job posting to the application form.
  `PrepareApplication` opens `job.CanonicalURL` and scrapes every `input`/`select`/`textarea` on whatever page that is; it clicks nothing, so it never presses "Apply" and never follows a posting through to the third-party ATS (Ashby, Greenhouse, Lever) that actually hosts the form.
  On both current connectors the canonical URL is the board's listing page, so this does not merely find nothing — it finds the wrong thing.
  Observed live on an Arbeitnow posting: the one field on the page was the site's own "Enter your email" job-alert signup, which the resolver answered with the candidate's contact address, the worker filled, and the run reported as `completed` before moving the application to READY_FOR_REVIEW.
  Filling a marketing form with the user's personal data and calling it a prepared application is a false success, not a missing feature.
- There is no vision or agentic page understanding in the browser worker.
  Field discovery is DOM heuristics only (the accessible-name chain, plus fieldset grouping); the LLM is used to answer narrative questions and, separately, to extract job details during custom-URL import, but nothing decides what to click or where to go next.
- The browser worker requires the user to log into any job board once
  inside its dedicated, JobPilot-owned Chrome profile (never their real
  daily-driver profile) before "prepare in browser" can get past an
  authentication wall on that board — there is no guided first-use flow
  for this yet.

## Next

1. The browser worker never navigates from the job posting to the actual application form (below), so "Prepare in browser" does not yet do what its name says on either public board.
2. Phase 8 (autopilot policy engine), the last unstarted roadmap phase.
   Before it, entity normalization (below) deserves an ADR and a decision: a policy engine that decides whether to submit on the user's behalf should not have to reason over JSON-blob candidate data.
3. The remaining documented gaps — structured LLM output, richer answer derivation, the browser worker's CAPTCHA heuristic, radio grouping fallback and salary fields — and the Settings categories still open (prompt customization, notifications, data retention).
