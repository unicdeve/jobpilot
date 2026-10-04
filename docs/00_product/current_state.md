# Current Development State

## Phase

Roadmap Phases 0–6 (`docs/00_product/roadmap.md`) are substantially
implemented — foundation, candidate profile, job discovery, matching,
application generation, a desktop app, and a browser automation worker —
with explicit gaps noted below.
Phase 7 (review mode) is complete: application review state and approval UI came with Phases 4 and 5, browser handoff with Phase 6, and submission tracking is described below.
Phase 8 (scouts and eligibility rules) is implemented: scouts run from the desktop or the CLI, or on a schedule the operating system's scheduler keeps (ADR-0027).
Phase 10 (distribution) is decided by ADR-0029 and under way.
Built so far: CI on every push and pull request (`.github/workflows/ci.yml`), which runs `make check` on Linux, both test suites on macOS and Windows, and builds and lints the desktop on all three; `make check` now vets and lints the Go code as each of the three platforms.
Also built: the `io.github.unicdeve.jobpilot` identifier.
Not yet built: versions and replacing an older daemon, alarms that remove themselves, installers, and the release workflow (`docs/08_development/packaging.md`).

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
- Go MarkItDown direction agreed, narrowed by ADR-0016: PDFs are extracted by JobPilot, everything else stays with MarkItDown
- Assist/Review modes agreed; Autopilot removed by ADR-0014, with unattended work moved to a Scout
- Review is default
- Cross-platform target agreed

### Features implemented (CLI unless noted; see gaps below)

- Saved-search default changes and deletion with replacement promotion are transactional, preserving the previous state on failure.
  Criteria edits preserve a concurrently changed default.
- A hosted model (ADR-0030, amending ADR-0019): "Custom" takes a provider's base URL as published (`https://openrouter.ai/api/v1`), a model, and an API key chosen from the model's keys or stored there and then, in Settings → AI model, the first-run wizard and `jobpilot-cli settings set-llm`.
  The key is a credential under the source `llm`; the settings hold its label, the daemon reads the key from the keychain each time it builds a request and sends it as a bearer token, and a key is refused for plain http to anything but this computer.
  A base URL that is not this computer shows, beside the field, that the profile, resume and job postings are sent to its host.
  Errors are written from the response status, never the body, which can quote the key.
  The OpenAI-compatible adapter now appends `/chat/completions` rather than `/v1/chat/completions`, and migration `00024` adds `/v1` to stored llama.cpp, LM Studio and custom base URLs so local servers keep working.
- Notifications (ADR-0028, `docs/02_features/11_notifications.md`): a scheduled scout run that created applications or failed, and any run that created an application for a strong match (80 by default), is recorded in a `notifications` table (migration `00023_notifications.sql`) and posted by the desktop app, which the daemon starts in a windowless delivery mode so the notification is JobPilot's own on every platform.
  The text is counts and the scout's name, written in core; a claim makes a notification post at most once; one never posted waits on the Dashboard under "While you were away".
  Settings → Notifications and `jobpilot-cli settings notifications` turn posting off and set the strong-match score; `jobpilot-cli notifications` lists what is new.
  A scout run is now recorded before it loads its saved search and the profile, so one that cannot start is a failed run in Activity, not an invisible one retried at every alarm.
  Email is designed for and not built.
- Scout schedules (ADR-0027, `docs/02_features/10_scouts.md`): a scout runs every N hours or daily at a local time, started by launchd, a systemd user timer or Task Scheduler whether or not JobPilot is open.
  An alarm runs `jobpilot scout wake <id>`, which finds or starts the daemon and asks `POST /v1/scouts/{id}/wake`; core decides whether the scout is due, and a due run belongs to the daemon.
  Runs happen one at a time, which also stops two overlapping runs creating the same application twice.
  The `scouts` table is the schedule: alarms are reconciled to it after every change, at start-up, and before "delete all data", by fingerprint, and a scout shows when it next runs and anything stopping its alarm.
  The desktop's scout editor and list, `jobpilot-cli scout schedule`, and `trigger` on every run (`started_by`, migration `00022_scout_schedules.sql`) say which runs were the schedule's.
  Measured on macOS with a real launchd agent; Linux and Windows are built and unit-tested, not yet run.
  Building it fixed three faults: editing a scout erased its last run, a finishing run rewrote the whole scout row, and the CLI cut every request off at 60 seconds, cancelling a long scout run or letter in the daemon.
- Scout cancellation preserves completed applications and finalizes the run with a separate five-second deadline, including cancellation during the final scoring call.
  Best-effort persistence failures are logged rather than silently discarded.
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
- Calendar dates (work history start/end, earliest start date) are a
  `domain.Date`, spelled `YYYY-MM-DD` everywhere — JSON, SQLite, and the
  UI. They were `time.Time`, which accepts only RFC 3339, so the desktop's
  date inputs could not save a work history at all; see ADR-0015's
  "Correction: a date is not an instant". Rows written in the old format
  still read, so no migration was needed.
- The desktop's resume picker offers only the formats that convert (PDF,
  DOCX, TXT, MD, HTML) rather than accepting any file and failing later.
- Document conversion is a port (`ports.DocumentConverter`, ADR-0016), so
  core no longer constructs a conversion library and `ResumeService` is
  testable without a PDF engine. The adapter
  (`internal/adapters/secondary/documents`) sends PDFs to PDFium's
  page text and everything else to Go MarkItDown, routing by file
  signature rather than by extension. MarkItDown's own PDF converter reads
  text out of text rectangles, which both duplicates and deletes
  characters: measured on a real resume it agreed with a reference
  extraction on 61% of tokens against 96% for the path now used, and it
  broke the candidate's name and email address. PDF hyperlinks — which are
  annotations, not text — are recovered and paired with the text they sit
  on, so a link to an employer or an open-source project is not read as the
  candidate's own.
- Resume management: import, list, delete, with an
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
- Job discovery: search and list against seven real, no-auth public sources —
  Arbeitnow, Remotive, RemoteOK, Himalayas, Hacker News's monthly
  "Who is hiring?" thread, We Work Remotely and Jobicy — and the hiring
  pages of the companies the candidate watches, with canonical-URL
  deduplication. None needs credentials, so discovery works on a fresh
  install with nothing configured. The last two were added by ADR-0023, and
  Himalayas is searched by title, keywords and country rather than read from
  its newest pages. Himalayas is the only one that reliably reports a salary range
  with its currency and period; Hacker News carries postings that exist on
  no job board, read from a convention nothing enforces, where a comment
  that does not fit is skipped rather than guessed at. Its header fields
  are reduced to plain text, because a company written with its homepage
  had been stored with the link inside its name; migration
  `00013_hackernews_company_links.sql` repaired the rows already stored.
  Google is deliberately not among them: a request to google.com/search
  returns a JavaScript gate and zero result links, and getting past it is
  the one thing ADR-0013 and ADR-0014 say the product never does. The
  Programmable Search API that would have been the legitimate route is closed
  to new customers and ends on 2027-01-01, so no Google source will be built;
  Brave Search API is deferred as the alternative (ADR-0020). Each connector caches its fetched
  listing in memory (15 min for Arbeitnow, 4 hours for Remotive, 30 min for
  RemoteOK, 1 hour for Hacker News, We Work Remotely, and each Himalayas
  query and Jobicy feed) matching
  that API's own stated update cadence, so repeated searches don't poll
  faster than their data actually changes; job descriptions (HTML) are
  converted through the same document converter resume import uses. Filtering
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
  optionally filtered by keywords over title and company, source, remote
  type and posting date (see AC-006), and
  returns a `{jobs, total}` envelope so a client can tell whether more pages
  remain. It reads local history only and contacts no source, which is what
  makes AC-006's "existing history remains usable offline" true in practice:
  previously the only way to see a job was `POST /v1/jobs/search`, which
  queries every source over the network and writes what it finds, so both
  `jobpilot-cli search list` (despite its help text) and the desktop Jobs
  screen re-ran a live search just to show a list. Both now read stored
  history instead.

- Source failures and being offline (`docs/02_features/04_job_discovery.md` AC-006, AC-008): a search reports how every source answered, in a `{jobs, new, sources}` result, instead of logging a failing source and carrying on.
  Before, an offline search failed at all five sources, logged each, and told the user it had found 0 jobs.
  Connectors now mark a source that gave no answer at all with `ports.ErrSourceUnreachable`; when every source is unreachable the search fails with a message that says finding new jobs needs internet access (503), and when every source failed with at least one reached it names each (502).
  Some sources failing is still a result, and the CLI, the desktop's Discover menu (a warning toast) and a Scout's trail (`source_unreachable` and `source_failed` steps) each name them.
  What a source said is logged and never returned, since a transport error carries its URL and a keyed source's URL carries its key.
  Sources are asked at once, so a search that cannot reach them fails after the slowest timeout (30 seconds) rather than after all of them in turn (100); a connector that panics is one failed source rather than a crashed daemon.
  A search also counts the jobs that are new to it, so a repeated search no longer reads as having found everything again.

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

- Credential storage (ADR-0019): API keys live in the OS keychain, never in
  the database, addressed by a source and a user-chosen label —
  `adzuna/work`. Managed from `jobpilot-cli credentials` and the desktop's
  Settings → Job source keys.

  A credential goes in and never comes out. There is no route that returns a
  secret, and `TestNoResponseCarriesACredential` checks that every response
  about credentials is free of one. The reason is ADR-0012: the daemon has
  no authentication, so a readable secret would be less protected on
  loopback than in a file with `0600` on it. The desktop says so where the
  key is typed rather than appearing to have lost it.

  Two places that would seem to need a secret still avoid one. A job board
  login lives in the browser worker's own Chrome profile, which the user
  signs into by hand; a cloud LLM credential belongs to Ollama, which holds
  its own, and `domain.LLMSettings` has no key field.

  `credential_names` (migration `00011`) records which refs exist and holds
  no value — `go-keyring` cannot enumerate, and a user-chosen label cannot
  be guessed. Writes go to the keychain first, so a crash leaves a stored
  secret nothing lists rather than a listed one nothing can read.

  Running the daemon with no login keychain reachable was measured, and it
  did not fail: macOS `security` waited for a prompt nobody would answer,
  indefinitely, and the request never returned. Every keystore call is now
  bounded at ten seconds and names the `JOBPILOT_CREDENTIAL_<SOURCE>__<LABEL>`
  override, which is read before the keychain and never written. A keystore
  that did not answer is a 503; the first implementation said 400, which
  told the user to fix a request that was correct.

  Only macOS is verified. The Linux and Windows paths are the library's.

- Search templates (`docs/02_features/14_search_templates.md`): a catalog of
  ready-made criteria, 34 of them in 7 groups, served from
  `internal/core/domain` through `GET /v1/search-templates` so the desktop
  and the CLI cannot drift apart. Importing one creates an ordinary saved
  search; a second copy of the same template is numbered rather than
  refused, and nothing links a search back to where it came from.
  Offered first in onboarding, and behind "Use a template" in Settings →
  Job searches, alongside `jobpilot-cli search templates list|import`.

  The catalog exists because `SearchCriteria.Matches` is unforgiving in ways
  the form does not show, and every entry was chosen by measuring candidate
  criteria against 654 live postings pulled from all five sources at once.
  Three findings shaped it: a role word in `Title` costs the half of the
  market that spells it the other way (`title="backend engineer"` and
  `title="backend"` returned the same 11 postings, and the second also finds
  Developer); a split compound beats any one spelling, since terms are
  matched as substrings (`"full stack"` 26, `"full-stack"` 15,
  `"fullstack"` 9); and `Keywords` reads whole descriptions, where a short
  word is supplied by prose (`keywords="go"` matched 426 of 654, on "go-to",
  "goals" and "Django"; `"rust"` matched 151, on "trust"). The same
  substring rule rules out searching on UI, UX, ML, SRE or iOS by name —
  "ui" matches "Cruise", "ux" matches "Luxury", "ml" matches "Teamleiter" —
  so each is spelled out or replaced.

  Measured for the profile that prompted it, the three specialist searches
  written by hand found 26 postings and the equivalent templates 112. The
  templates are guarded by tests rather than by review: every one must match
  a real posting, none may match a decoy, and no keyword may be shorter than
  five characters.

- Custom job URL import (`docs/02_features/04_job_discovery.md` AC-007):
  none of the five public APIs can fetch an arbitrary job by
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
- Data deletion and "Not interested" (`docs/02_features/15_data_deletion.md`, ADR-0021): a job, an application, a cover letter version, a resume, the profile, the activity history, the unused jobs, or everything, from the CLI and the desktop.
  Before, almost nothing could be deleted: twelve of the eighteen foreign keys had no `ON DELETE` rule, so a row anything referenced could not be removed, and deleting a resume an application had used failed with a 500.
  Migration `00015` rebuilds the eight tables concerned so every foreign key carries its rule, and repairs, by those rules, the orphans left from when foreign keys were enforced on one pooled connection only.
  The rules are `domain.DeletionRules`, and a test fails if the schema's foreign keys ever disagree with them.
  Every deletion is previewed first, by kind and count, and carried out only with the preview's token; one whose reach changed since is refused with the new reach, so a scout acting between the two cannot have something deleted the user was not shown.
  Deleted content is overwritten on disk (`secure_delete`, a WAL checkpoint after each deletion, and a vacuum after "delete all data"), which also removes the keychain's API keys, the browser profile and the daemon's log.
  "Not interested" hides a job from every list and search result and has scouts pass it over before scoring, without deleting it; `jobpilot-cli job not-interested`, and a button on a job's page and in the job list.
  The repository `Delete` methods the deletion service supersedes were removed, so nothing deletes without a preview, and core no longer imports the SQLite adapter to set up storage (`ports.LocalStorage`).

- No profile yet is an answer, not an error: `GET /v1/profile` is a 404 for an installation that has none (`ports.ErrNoCandidate`), and the desktop reads it as `null`.
  It was a 500, which the desktop kept as an errored query, and that turned into a request loop that left the app on "Loading…" for anyone who skipped first-run setup.
  An errored query with no data reads as pending while it refetches, the setup gate unmounted every screen while the profile was pending, and each screen remounting refetched it — thousands of requests a minute against a fresh install.
  The gate now waits only for the first answer, which also stops a slower form of the loop while the daemon is down.
  An error from the daemon now reads as a sentence, without the "Error: " that had been in front of every one.
- A record an ID names that does not exist is a 404 across the API, not a 500 (`docs/01_architecture/07_ipc.md`, "A missing record is 404, a missing reference 422").
  Every SQLite repository answers a missing row with `ports.NotFoundError`, which matches `ports.ErrNotFound`, and the HTTP layer's one fallback maps it.
  The desktop's job, application and activity-run pages show that the record is gone, with the way back to the list, instead of the daemon's message in red, and do not retry a 404.
- A record a request body names that does not exist is a 422 (`docs/01_architecture/07_ipc.md`, "A missing record is 404, a missing reference 422").
  The service looks it up before saving, so `POST /v1/applications` and `POST /v1/resumes` with a job, candidate or resume that is not there no longer fail at the database's foreign key as a 500, and match analysis, watching a company from a job, and saving a scout answer the same way instead of with 404 or 400.
  A `PUT` to a scout that does not exist is now 404 rather than 400.

- Desktop first-run setup (`/welcome`, `docs/02_features/01_onboarding.md`):
  the four things an installation needs — LLM provider, candidate profile,
  optional resume, saved search — in the CLI `onboard` order, since
  prefilling a profile from a resume needs the LLM. A new installation is
  redirected there; skipping is remembered for the session and a banner
  leads back while anything is still missing. Progress is derived from what
  exists rather than from a stored flag, so it resumes at the first gap.
  Previously each prerequisite was reachable on its own screen and each
  screen said what was missing, but nothing put them in an order.
- Setup ends by searching (`docs/02_features/01_onboarding.md` AC-006): the searches the desktop wizard adds are run as it finishes, so it no longer ends on a dashboard of zeros with a Discover button to find.
  A search while it runs is shown at the bottom right ("Searching 5 job sources…", and which search), from `GET /v1/sources`, and says what it found when it is done.
  Discovery runs are queued one at a time wherever they were started, and a run outlives the screen that started it; the Discover menu cannot ask for a search already going.
  The Discover menu's "All saved searches" queues every saved search at once, rather than one click each.
  The desktop no longer pauses its requests to the daemon when the OS reports being offline: TanStack Query's default does that, and the daemon is on loopback, so it stopped local history loading at exactly the time it is meant to keep working.
- About you (`docs/02_features/16_about_you.md`, ADR-0022): the candidate says who they are — six optional questions, how they want to sound, a writing sample — once, and a few words about each job before its letter: what draws them to it, and up to two questions the model proposes from the posting.
  Letters and written answers are built from it; written answers also get the resume, which they lacked.
  A written answer the candidate wrote can be kept for similar questions.
  Only the candidate writes an answer; the model only proposes questions, which are checked and delimited as untrusted.
  Stored in `personal_answers` (migration `00018`), which also took `application_preferences` from the candidate.
  Desktop: Profile → About you, and a questions step before "Write one" and "Rewrite". CLI: `profile about`, `cover-letter generate` asks first, `question keep`.
  A rewritten letter now replaces the old one on screen straight away; it used to show until the application refetched.
- Checking what the model wrote (`docs/02_features/19_draft_checks.md`): every cover letter and generated answer the API returns carries flags for the reviewer — stock phrases, a letter over the length its writing preferences give (or 250 words), and numbers of years the candidate never gave.
  `domain.CheckDraft` is deterministic and involves no model; a claim naming a skill is held to the duration the candidate gave it, one naming none to their dated work history, and either stands when the candidate's own words — resume, profile, About you — say it.
  Flags are worked out on every read and never stored, so an edit is re-checked at once, and they block nothing.
  Desktop: the flagged words are marked in the letter and answer, with a "To review" list beneath. CLI: the same list after `cover-letter generate`, `list` and `edit`, and `question resolve`, `answer` and `list`.
- Generation is streamed from Ollama and OpenAI-compatible servers, and given up on only after 60 seconds without a byte (five minutes before the first, for a model loading from disk), with no limit on the total.
  The 30-second limit on every request is gone: a reasoning model run locally, measured with `deepseek-r1`, does not finish a letter inside it.
  A stream cut off before the model says it is done is an error rather than a shorter letter, and an OpenAI-compatible server that answers in one piece is read that way.
- Where a job can be done from (ADR-0023, `docs/02_features/04_job_discovery.md` AC-010 to AC-013): every job says whether it can be done from anywhere, from listed countries, or does not say, read deterministically from its locations against compiled country, region and city tables from GeoNames (`internal/core/domain/geo`); a title or sentence that contradicts its source's label makes it "not stated", with the sentence shown.
  Every job also says whether it offers, mentions or refuses relocation and visa sponsorship, with the sentence each was read from.
  A saved search has "I can work from" (a country) and "also include jobs elsewhere that offer relocation or visa sponsorship"; a job refusing sponsorship is kept only for a candidate whose profile says they can already work there.
  New searches, from the form or templates, start with the profile's country. The stored list filters the same way, in SQL held to `SearchCriteria.MatchesFor` by a test.
  Stored on the job (migration `00019`) and read again for every stored job when the daemon starts, whenever the reading's version changes.
  Desktop: the saved-search form, the Jobs filter "Where you can work", readable locations and offer marks in the list, and "Where you can work" on a job's page. CLI: `--work-from` and `--include-relocation` on saved searches and `search list`, and `search show` prints both.
- Company watchlists (ADR-0024, `docs/02_features/17_company_watchlists.md`): the candidate watches a company by the address of its Greenhouse, Lever or Ashby hiring page, or with "Watch this company" on a job whose posting is on one, and discovery reads every watched company's page as one more source, "Company pages", under the same saved-search criteria as every board.
  Each page is read at most four at a time and once an hour, each check is recorded on its company, and a company whose page has gone is named in what a search says while the other sources' jobs are kept.
  A starter list of 38 companies to suggest is generated by `make starter-list` from a seed of 87 boards, each entry saying how many of its open roles could be done from anywhere or from more than one continent when measured.
  Stored in `watched_companies` (migration `00020`), deleted with "delete all data" and not with the profile.
  Desktop: Settings → Job searches → Companies you watch, with suggestions, and "Watch this company" beside a job's company. CLI: `company watch|list|rename|unwatch|suggestions`.
  Building it fixed ADR-0023's reading of "Georgia" beside the United States and of airport codes read as cities, and taught it "AMER"; stored jobs are read again for it.
- Workable and SmartRecruiters company pages (ADR-0026, proposed): watched as the other three are, by `apply.workable.com/…`, `{account}.workable.com`, `careers.smartrecruiters.com/…` or `jobs.smartrecruiters.com/…`.
  Workable's short posting address, `apply.workable.com/j/…`, names no company and is followed to the one that does when pasted.
  SmartRecruiters lists postings without descriptions, so only the postings a search may keep are described, one request each, and kept in memory by posting.
  A Workable refusal to answer (429) names the company as failing for that search, never as gone.
  Both give one place per posting, so a regional remote role reads as one country, and neither reaches the starter list: none of 13 SmartRecruiters companies, or of the 18 Workable accounts that answered, qualified.
- Working hours (ADR-0025, `docs/02_features/18_time_zones.md`): every job says the time zones it requires or prefers, as one window of UTC offsets read deterministically from its description — "CET (+/- 3 hours)", "UTC−6 to UTC+3", "overlap at least 4 hours with US Eastern", "9:00 AM – 12:00 PM PST", "home based in EMEA time zones" — with the sentence shown; sentences that only mention working across zones are not read.
  The profile gains a time zone (an IANA zone, from GeoNames' table of 418, compiled into `geo`), prefilled from the country when it has one zone and from the computer otherwise.
  A saved search can leave out jobs whose required hours the candidate cannot work, allowing them to move their day by up to 12 hours they choose; preferences never leave a job out.
  Stored on the job, profile and saved search (migration `00021`), and every stored job is read again.
  Desktop: Profile → Location → Time zone, the saved-search form, an hours mark on each job in the list and an hours line on its page. CLI: `profile update`, `--fit-hours` and `--hours-shift`, and `search show`.
- The desktop refreshes stale searches when it opens (`docs/02_features/04_job_discovery.md` AC-009): each saved search records when it last ran (migration `00017`, from any path that ran it and got an answer), and on opening the desktop runs those not run in the last six hours, through the Discover queue, then says what they found in one line.
  It is once per launch and the desktop's, never the daemon's on starting, since a CLI command starts the daemon too.
  Settings → Job searches turns it off, and shows when each search last ran.
- Hot reload for the daemon (`make dev`, `docs/08_development/hot_reload.md`): air rebuilds and restarts it when its Go or migrations change, pinned in the Makefile and installed into `./bin`.
  The desktop now follows a daemon that restarts: it used to resolve the port once, so any restart left it pointed at a closed one until the app was reopened.
  It waits for a daemon to answer without starting one, and a daemon refuses to start while another answers, so a restart cannot leave two on one database.
  A daemon shutting down removes the discovery file only if it still names that daemon.
  The desktop's daemon commands run off the main thread; starting a daemon had frozen the window for as long as it took.
- The app identifier is `io.github.unicdeve.jobpilot` (ADR-0029 §1), replacing the Tauri template's `com.user.jobpilot`, and the Rust and npm packages are `jobpilot-desktop`.
  The package is not `jobpilot`, which is the daemon's binary and what the app looks for beside its own executable.
  The webview's storage is keyed by the identifier, so the theme preference is chosen again once, and macOS asks again for permission to notify.
  The macOS agents are named after it too.
  An agent under the old prefix is listed with no fingerprint, so the first reconcile rewrites it under the new one; a Scout the user had switched off in Login Items stays off, which `launchctl disable` says again for the new label.
  The old prefix is recognised until the first release, then removed.
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
- Candidate profile normalization, stage one (ADR-0015, migration
  `00007_normalize_candidate_facts.sql`): email and phone, city/region/country,
  notice period and earliest start date as columns; links and per-country work
  authorization as their own tables (`candidate_links`,
  `candidate_work_authorizations`), written with the candidate in one
  transaction.
  This exists because the resolver was measurably wrong, not merely thin.
  Run against the question labels recorded from live Ashby, Greenhouse and
  Lever forms, the old matcher put the candidate's email address in both
  forms' phone fields, put every one of their URLs into each of LinkedIn,
  Portfolio and Website while leaving GitHub and Twitter empty, and answered
  a Yes/No sponsorship question with a paragraph of prose.
  `profileFactAnswer` is now an ordered rule table that refuses
  decision-shaped labels ("Are you…", "Do you…"), work authorization is
  answered only for a country the candidate has spoken about and the job is
  actually in, and a question offering fixed choices is answered only with
  one of them — an answer a `<select>` does not have used to fail the whole
  run. `internal/core/services/answers_forms_test.go` asserts all of it
  against the recorded labels.
- Candidate profile normalization, stage two (ADR-0015, migration
  `00008_structured_experience_and_skills.sql`): the work history as dated
  entries and skills as a list with optional durations, replacing two prose
  columns.
  This is what makes answers possible that could not be given at all:
  "Current company", asked by two of the three recorded forms, and a years-of-experience
  figure measured as a span rather than summed, so two jobs held at once are
  not counted twice. A question naming a skill is answered only from what the
  candidate said about that skill — answering "how many years of Rust?" with
  the decade they spent writing Go is the kind of invented fact principle 6
  forbids. Resume extraction was widened to propose all of it, review-gated
  under ADR-0011, and is forbidden by its own prompt from estimating a
  duration the resume does not state.
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
  Screens: Dashboard, Jobs, Applications (the review/approval UI — state
  transitions, cover letter versions, question resolution), Scouts,
  Activity, Profile, Resumes, Settings. Accessible components are generated
  via the shadcn/ui CLI, per `02_desktop.md`.
  The look and feel follows the rules in `02_desktop.md`: colours are theme
  tokens only, with a light and a dark theme and a System / Light / Dark
  choice under Settings → Appearance kept in the desktop's own storage; the
  platform's own font; a grouped sidebar with ⌘1–⌘7 and ⌘, shortcuts and a
  count of applications waiting on the user; and Jobs, Applications and
  Activity as split views, a list beside the selected item. On macOS the
  title bar is drawn over the sidebar (`src-tauri/tauri.macos.conf.json`).
  Matches is no longer a screen of its own: a match is a job with a score,
  ranked under Jobs → Analyzed, and `/matches` redirects there.
  Not built: any settings category beyond appearance, the model, job
  searches and API keys (unimplemented backend).

- Agent Activity (`docs/02_features/12_agent_activity.md`): the desktop's
  Activity screen lists every agent run — browser preparation and scout runs
  alike — newest first, filterable by kind and status, and opens each one to
  show its steps with their policy results. `GET /v1/agent-runs` and
  `GET /v1/agent-runs/{id}` serve it, read by `AgentActivityService`, and
  `jobpilot-cli activity list` / `activity show` read the same trail.
  A scout run now records its scout (migration `00012_agent_activity.sql`),
  and steps decided by a deterministic rule record its verdict as their
  policy result. A filled field's step no longer records the answer typed
  into it; the same migration removed the answers already stored.

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

- Job filters: a filter button beside "Add a job by URL" on the Jobs screen.
  All jobs filters by posting date, source and work type, sent to the
  daemon since that list is paged there. Analyzed filters by match score,
  when the job was analyzed, source and work type, applied in the desktop,
  which already holds every match. Filters live in the URL like the search.
- Stored times: from migration 00014 (a Go migration, `migrations_go.go`)
  every DATETIME is written in UTC as `2006-01-02 15:04:05.999999999-07:00`
  (`_time_format=sqlite`, `_timezone=UTC` on the connection), which SQLite's
  date functions read. Before it the driver wrote `time.Time.String`, in
  the local zone and with a monotonic-clock suffix, so no query could
  compare dates. The migration rewrites existing values and refuses to
  guess at one it cannot read.

- Desktop jobs list and scroll restoration: the Jobs screen pages through
  stored jobs with `useInfiniteQuery` over `GET /v1/jobs` (50 at a time,
  fetching the next page when a sentinel scrolls into view, with a real
  "Load more" button behind it for keyboard users and for the case where the
  observer never fires). Sections stay alive between visits: the shell
  hides the ones not showing with React's `<Activity>` instead of
  unmounting them, so selection, search, drafts, open dialogs and scroll
  positions survive navigation with no restoring code — see "Sections and
  navigation state" in `docs/01_architecture/02_desktop.md`. The sidebar
  and ⌘1–⌘7 return to where a section was left.

- Job-centric desktop navigation (`docs/01_architecture/02_desktop.md`): a
  job detail route (`/jobs/:id`, served by `GET /v1/jobs/{id}` and
  `jobpilot-cli search show`) shows a stored job with its match, its
  applications, and the actions that create either. Jobs, the analyzed-jobs
  view and Applications all link to it, and applications show the job's
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
  the job posting, walks from there to the page that actually holds the
  application form, extracts it (label association follows the same
  chain browsers use: `<label for>` → wrapping `<label>` → `aria-label` →
  `placeholder` → name, reading what is rendered rather than what the markup
  holds, with radio buttons sharing a name grouped into one
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
  The route from posting to form is ADR-0013, now complete.
  `services.PickApplyControl` ranks the page's own links and buttons — ATS destination, then URL path, then wording — and on every board measured so far it answers with no model involved.
  When it ranks nothing, `services.ApplyControlChooser` shows the model those same controls as untrusted content and asks for a position in the list or an abstention; it returns an index, never a URL, so the browser can only be sent where the page already pointed, and a missing, unreachable or incoherent model leaves the run exactly where it would have been without a fallback.
  Whoever chose, `services.LooksLikeApplicationForm` decides what was reached: a page counts only if it asks about a person in at least three distinct ways.
  That gate was not there before, and the absence had teeth — observed live on an Arbeitnow posting, the one field on the listing page was the site's own "Enter your email" job-alert signup, which the resolver answered with the candidate's contact address, the worker filled, and the run reported as `completed` before moving the application to READY_FOR_REVIEW.
  Every hop is recorded in the run trail with its destination, whether a model chose it, and whether it left the origin.
  Recorded copies of six real pages — Ashby, Greenhouse and Lever application forms, and Arbeitnow, Remotive and Ashby postings as the negatives — run the whole decision offline in `internal/adapters/secondary/browser/testdata/ats`, with what was extracted from each checked in beside it.
  `AgentRun`/`AgentEvent`
  (`internal/core/domain/agent_run.go`) get their first real implementation
  here (schema existed since migration 00001, unused until now), each run
  scoped to one application (migration `00003_agent_runs_application_link.sql`,
  which also adds `source`/`field_ref` to `application_questions`) and
  surfaced via `GET /v1/applications/{id}/agent-runs` and a panel on the
  desktop's Applications detail screen.

## Known gaps within Phases 0–6

- No settings/configuration system beyond LLM provider/model, job
  searches and notifications (`docs/02_features/13_settings.md` — other
  categories like prompt customization and data retention are unimplemented).
- No job source that needs an API key is *built* yet, though credential
  storage now exists for one (ADR-0019, below). Measured, Adzuna, Jooble,
  USAJOBS and The Muse all refuse an unauthenticated request.
- No user-configurable prompt customization for matching (Phase 3).
- The desktop has no progress indicator for a long import or discovery
  beyond its button's label; a resume read by a slow model shows
  "Reading…" for as long as it takes.
- Some core entities from `docs/01_architecture/04_database.md` are still
  not separate tables: `JobSource` (jobs reference their source only by
  free-text ID) and `Prompt` (no schema home yet for prompt customization).
  The candidate half is done — `candidate_skills`, `candidate_experiences`,
  `candidate_links` and `candidate_work_authorizations` are real tables.
  `CandidateEducation` is deliberately not among them; see the education
  entry below.
  ADR-0015 covers the candidate half and is complete. Stage one normalized
  contact, location, links, work authorization and availability; stage two
  added dated work history and skills; the closing change made work
  preference structured and removed education and certifications, which no
  interface ever wrote and nothing ever read.
- Structured model output is schema-constrained where the backend supports
  it (ADR-0017). `ports.Request` carries an optional `ResponseSchema`, which
  Ollama compiles to a sampling grammar; llama.cpp, LM Studio and custom
  endpoints ignore it until each is measured, so they behave as before.
  The schema type cannot express `enum`, `pattern` or numeric bounds: the
  latter two are not enforced by a grammar, and `enum` is enforced in a way
  that turns a detectably wrong answer into a plausible one. Every
  structured reply is validated after decoding regardless, which is new for
  the job-page extraction — it previously checked nothing but a non-empty
  title, on content scraped from an arbitrary web page.
- The profile is empty until it is filled in. ADR-0015 dropped the prose
  columns, so after upgrading JobPilot answers less than it used to until the
  user enters their details or re-imports their resume. That is the intended
  direction — an unanswered question is left for the user, a wrongly answered
  one goes to an employer — but it is a real regression in the interim.
- Education and certifications are not stored at all. No recorded form asks
  about them, so bringing them back should start with a fixture that does.
- Per-skill durations are self-reported and never estimated. A question about
  a skill the candidate has not given a duration for is left for them, rather
  than answered from their career total.
- Scouts are implemented (ADR-0014, ADR-0018): a scout runs a saved search,
  scores what it finds through the existing match service, and creates
  applications in Review state for the user to read. It stops before the
  browser — `ScoutJobFinder` exposes only search and list, so no scout code
  path can reach `ImportJob`, which can open one. Eligibility is one
  deterministic function in core that returns the reason with the decision,
  because the run's trail has to record why something was rejected.
  Three of ADR-0014's five rules were already built: salary floor, excluded
  companies and location/work-arrangement live on the saved search and are
  applied by `SearchCriteria.Matches`. A scout adds only a match-score floor
  and a per-run ceiling.
  A scout is started by the user or by its schedule (ADR-0027). Unattended
  preparation in the browser stays deferred.
  `ApplicationMode` is now Assist and Review only, and an unrecognised mode is refused at the boundary rather than stored (ADR-0014, migration `00006_retire_autopilot_mode.sql`).
  "Assist does not interact with external application forms" is enforced in core: `BrowserWorkerService.PrepareApplication` refuses an Assist application before a browser opens (`409` over HTTP). It had been stated and not checked.
- The browser worker's CAPTCHA/anti-bot detection (`internal/adapters/secondary/browser/playwright_worker.go`) is a best-effort heuristic, not a real detector, and can still miss a novel challenge — which then surfaces as an extraction or fill failure instead, the same safe stop.
  It now asks whether a challenge is being posed rather than whether a vendor's script is present: a CAPTCHA iframe blocks only when it is visible and tall enough to cover the form, and the interstitial phrases are matched against rendered text rather than page source.
  Both of those were false positives with teeth. Real ATS forms carry an invisible reCAPTCHA (measured on Ashby: 256x60, `visibility: hidden`) while being entirely fillable, and the words appear in the scripts of pages that work perfectly well, so the previous "any frame URL containing recaptcha" rule would have failed every application the worker could reach.
- Radio button groups are extracted as one question with the group's
  `<fieldset><legend>` text as its label, falling back to a humanized
  `name` attribute when there's no fieldset/legend — real-world forms
  without either markup pattern will show a rough, not fully human-authored
  label.
- Salary fields on extracted forms aren't filled: `ApplicationQuestion` has
  no structured salary type, so a form's salary field becomes an ordinary
  text question resolved (or not) like any other, rather than something
  purpose-built for numeric/range salary inputs.
- There is no vision or agentic page understanding in the browser worker.
  Field discovery is DOM heuristics only: the accessible-name chain, fieldset grouping, and a readability filter that drops a question whose only label is a machine identifier.
  The model's single job in the browser loop is choosing an apply control when nothing else can (below); it never decides what a field means.
- Some real questions are dropped rather than asked.
  A field whose label is a UUID — Lever's custom cards, Ashby's ungrouped radios — is skipped as unanswerable, and the count of skipped fields is not reported anywhere.
  The user is not told, beyond the browser window being left open on a form with blanks in it.
- The browser worker requires the user to log into any job board once
  inside its dedicated, JobPilot-owned Chrome profile (never their real
  daily-driver profile) before "prepare in browser" can get past an
  authentication wall on that board — there is no guided first-use flow
  for this yet.

## Next

1. Running scout schedules on Linux and Windows. Both adapters are built and tested on the systemd units and task XML they generate, but neither has run on its system. Linux needs a systemd user manager, and a session without one says scheduling is unavailable. On Windows, `conhost.exe --headless` keeping the alarm windowless, and the daemon it starts outliving the task, are both unmeasured.
   Beside it, seeing a notification from a built, bundled app: delivery is measured with the debug binary, which macOS attributes to the terminal, and Windows attributes a toast to PowerShell until there is an installer (ADR-0028).
   Both wait on the rest of Phase 10 (ADR-0029): installers, and alarms that remove themselves once JobPilot is uninstalled. Until then an uninstalled JobPilot leaves its alarms behind.
2. The remaining documented gaps — richer answer derivation, the browser worker's CAPTCHA heuristic and radio grouping fallback, the skipped fields a user is never told about, and what the draft check leaves out ("Never mention", and claims other than years: `docs/02_features/19_draft_checks.md`, "Out of scope") — and the Settings categories still open (prompt customization, data retention).
   Two are no longer among them: ADR-0015 §4 settled salary, with the filter on the saved search and a form's salary question left for the user, and ADR-0017 settled structured LLM output.
3. A keyed job source, now that there is somewhere to put a key (ADR-0019). Adzuna is first, once there is a key to build it against; USAJOBS and Jooble are the same shape. Google's Programmable Search API, which prompted ADR-0019, is closed (ADR-0020), and Brave Search API is deferred as its alternative. Nothing yet says which label a run uses when a source has several — identity is settled and selection is not, and it becomes a real question the moment two labels can both be live.
4. Verifying the keystore on Linux and Windows. Only macOS was measured, and what was measured there was a hang rather than an error, so the other two should not be assumed.
5. A title naming a region wider than a job's one listed place: Hugging Face's "EMEA Remote" roles on Workable list Paris and read as France (ADR-0026 §4).
   Whether such a title should make the reading less certain is a change to ADR-0023 for every source, left open.
