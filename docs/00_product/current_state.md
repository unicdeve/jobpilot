# Current Development State

This page summarizes implementation and verification status.
Feature specifications define behavior, and [ADRs](../09_decisions/) retain the decisions and measurements behind it.
The [roadmap](roadmap.md) records the original phase scope.

## Phase

| Phase | Status |
| --- | --- |
| 0–6: foundation through browser preparation | Substantially implemented; limitations below |
| 7: Review mode | Implemented, including approval, browser handoff and user-reported submission/outcomes |
| 8: Scouts | Implemented, including deterministic eligibility, OS schedules and desktop notifications |
| 9: Extensibility | Deferred; the plugin protocol remains proposed |
| 10: Distribution | Packaging and release automation built; first release and installed-build verification outstanding |

Prompt customization (PR-013, ADR-0031) closes the last originally outstanding MVP requirement.
Implemented does not mean verified on every target platform or guaranteed to work with every job site.

## Architecture and boundaries

- Go daemon owns core services and SQLite; the Go CLI and Tauri 2 + React + TypeScript desktop are clients of its loopback HTTP API (ADR-0012).
- Data stays local by default, with no application account or mandatory hosted backend.
- Ollama is the default local model; llama.cpp, LM Studio and custom OpenAI-compatible endpoints are supported.
- Hosted model use sends candidate context and job content to the configured host, with a notice when configured (ADR-0030).
- Review is the default mode; Assist never prepares external forms.
- JobPilot never submits applications, in any mode or configuration; the user submits in the browser (ADR-0014).
- Scouts stop before the browser and create applications for review.
- Candidate facts, eligibility rules, answer derivation and draft checks remain deterministic where required; external content is untrusted.
- Go Playwright uses installed Chrome and a dedicated JobPilot profile; document conversion uses a port, with JobPilot's PDF extraction and Go MarkItDown for other supported formats (ADR-0016).

## Implemented workflows

### Setup, profile and resumes

- CLI onboarding and the desktop first-run wizard configure the model, profile, optional resume and saved searches.
  Setup resumes from existing data; desktop completion queues the added searches without requiring discovery to succeed offline.
- Candidate facts are structured: names, contact, location/time zone, availability, links, work authorization, remote preference, skills and dated work history (ADR-0015).
  Calendar dates use `YYYY-MM-DD`; experience spans do not double-count overlapping roles.
- PDF, DOCX, TXT, MD and HTML resumes can be imported, listed and deleted.
  Resume-to-profile suggestions require review and never directly overwrite saved candidate facts (ADR-0011).
- About you stores the candidate's own stories, preferences, writing sample and reusable personal answers, with per-application context (ADR-0022).

See [onboarding](../02_features/01_onboarding.md), [profile](../02_features/02_candidate_profile.md), [resumes](../02_features/03_resume_management.md) and [About you](../02_features/16_about_you.md).

### Discovery and matching

- Seven no-key sources are production connectors: Arbeitnow, Remotive, RemoteOK, Himalayas, Hacker News, We Work Remotely and Jobicy.
  Watched company pages support Greenhouse, Lever, Ashby, Workable and SmartRecruiters (ADR-0024; ADR-0026 remains proposed).
  Mock jobs are excluded from production discovery.
- Saved searches, a catalog of search templates and company watchlists are available in both interfaces.
  Criteria edits and default-search changes preserve concurrent or failed updates.
- Discovery normalizes and deduplicates jobs, caches source responses, counts newly stored jobs, and names partial source failures.
  An all-unreachable search reports that fresh discovery needs internet.
- Browsing stored jobs is a separate, paginated local read path, usable offline.
  Jobs can be filtered, hidden as not interested, imported by URL and opened with their analysis and applications.
- Core derives location eligibility, relocation/sponsorship signals and working-hour requirements from posting evidence (ADR-0023, ADR-0025).
  Saved searches can filter by work country, relocation/sponsorship and fitting hours.
- Matching returns validated structured scores, confidence, strengths, gaps and rationale, persisted locally.
  Ollama supports schema-constrained output; every provider's decoded output is validated (ADR-0017).

See [discovery](../02_features/04_job_discovery.md), [matching](../02_features/05_job_matching.md), [templates](../02_features/14_search_templates.md), [watchlists](../02_features/17_company_watchlists.md) and [working hours](../02_features/18_time_zones.md).

### Applications and browser preparation

- Applications have a validated state machine and durable transition history.
  Approval, external submission and subsequent outcomes are explicitly recorded by the user; no outcome is inferred.
- Cover letters are versioned and editable.
  Answers resolve through profile facts, deterministic derivation, narrative generation, or a request for user input; unknown factual answers never reach the model.
- Deterministic draft checks flag preambles, stock phrases, excess letter length and unsupported years of experience.
  Flags are recomputed on read and advise the reviewer without blocking actions.
- Model generation streams with inactivity limits rather than a fixed total duration; incomplete streams fail.
- Browser preparation shares one Chrome session, with a tab per task.
  Apply-control rules choose the route to a form, with a constrained model fallback; a form gate prevents filling unrelated signup boxes (ADR-0013).
- Resolved fields are filled; unanswered questions remain for the user.
  Success, detected blockers and failures leave the preparation browser window open, and no submit control is clicked.
  Custom job import closes its reading window after extraction.
- Recorded Ashby, Greenhouse and Lever forms and negative posting-page fixtures exercise extraction and routing offline.

See [letters](../02_features/06_cover_letters.md), [answers](../02_features/07_application_answers.md), [applications](../02_features/08_applications.md), [Review](../02_features/09_review_mode.md), [draft checks](../02_features/19_draft_checks.md) and [browser architecture](../01_architecture/03_browser_worker.md).

### Scouts, notifications and activity

- Scouts use a saved search, a score floor and a per-run ceiling to analyze local or freshly discovered jobs and create applications for review (ADR-0018).
  Runs serialize, skip ineligible or already handled jobs, and preserve completed applications on cancellation.
- Manual, every-N-hours and daily schedules are supported through launchd, systemd user timers and Task Scheduler (ADR-0027).
  The database is authoritative; core decides whether a wake is due and reconciles alarms after changes and daemon startup.
- Notification delivery records a claim to avoid duplicate posting; undelivered items appear on the Dashboard.
  Settings control posting and the strong-match score (ADR-0028).
- Desktop Activity and CLI activity commands show browser/scout runs, steps, failures and deterministic policy results.
  Filled-field events do not retain the candidate's answers.

See [Scouts](../02_features/10_scouts.md), [notifications](../02_features/11_notifications.md) and [Activity](../02_features/12_agent_activity.md).

### Desktop, settings and data

- Screens: Dashboard, Jobs (including Analyzed), Applications, Scouts, Activity, Profile, Resumes and Settings.
  Matches is part of Jobs; `/matches` redirects there.
- TanStack Query manages server state, including offline loopback access; sections retain selection, drafts and scroll positions between visits.
  Discovery is queued, shows source/search progress and can refresh stale saved searches once per launch.
- Light, dark and system themes, keyboard navigation, split views and safely rendered job markdown are implemented.
- Settings cover appearance, model/key selection, prompt guidance, saved searches/watchlists, notifications, API keys, data deletion and About.
  Prompt guidance changes judgement/style; safety rules and response contracts remain code-owned (ADR-0031).
- Credentials live in the OS keystore, with labels stored locally and no secret-read API.
  Keychain calls have bounded waits and documented environment overrides (ADR-0019).
- Deletion previews show dependent records and require an unchanged preview token.
  Foreign-key rules match core deletion rules; secure deletion/checkpointing covers SQLite, and delete-all also removes credentials, browser data, logs and Scout alarms (ADR-0021).

See [desktop](../01_architecture/02_desktop.md), [settings](../02_features/13_settings.md), [prompts](../02_features/20_prompt_customization.md) and [deletion](../02_features/15_data_deletion.md).

## Distribution and verification

Packaging follows [ADR-0029](../09_decisions/0029-shipping-jobpilot.md): `io.github.unicdeve.jobpilot`, Apache-2.0, unsigned initial builds and manual updates.
The app bundles the daemon and CLI; standalone archives cover macOS/Linux arm64 and amd64, plus Windows amd64.
Desktop packages are a universal macOS `.dmg`, Windows per-user NSIS installer, and Linux `.deb`/`.rpm` for both architectures.
Every bundle/archive includes project and third-party license notices.

Clients replace an older daemon safely, use a newer daemon without downgrading it, and wait on its process lock during startup/shutdown.
A daemon refuses a database with unknown newer migrations.
Windows install/uninstall hooks stop the daemon and remove/restore Scout alarms; macOS/Linux alarms remove themselves when the executable is gone.

CI is configured for pushes and pull requests targeting `main`.
Linux runs `make check-go`; desktop jobs on all three systems run Biome, frontend builds, Rust formatting, clippy and tests.
Go tests run on all three, with race detection where supported; notices are checked on Linux.
The release workflow validates tag/version agreement, runs CI, builds packages and checksums, and creates a draft release for manual publication.
It has not had its first release run.

| Runtime check | Measured | Outstanding |
| --- | --- | --- |
| Desktop bundle and sidecars | Local universal macOS app/DMG, ad-hoc signatures, app/CLI starting bundled daemon | Download quarantine/Open Anyway and installed builds on Windows/Linux |
| Daemon upgrades | Newer/older clients and desktop replacement on macOS | Runtime checks on Windows/Linux |
| Scout alarms | Real launchd run, self-removal and remove/restore on macOS; generated files/commands tested on all platforms | Real systemd and Task Scheduler runs, windowless Windows wake and daemon survival |
| Notifications | Debug-binary delivery | Bundled-app attribution and OS permission behavior |
| Keystore | macOS access and bounded failure when login keychain is unavailable | Real Linux/Windows keystores |
| Release automation | Workflow/scripts checked locally | First GitHub release build and downloaded-artifact installation |

See [testing](../08_development/testing.md) for check coverage and [packaging](../08_development/packaging.md) for the release procedure.

## Limitations and deliberate exclusions

- Other settings categories remain unimplemented: a global application-mode setting, retention policy, storage relocation, browser configuration and additional policies.
  Application modes and Scout eligibility can already be chosen on their own records.
- Resume/model work can still show only a busy label for a long operation; discovery has queue/progress feedback.
- Browser CAPTCHA detection and radio labeling are heuristic.
  Machine-identifier-only fields can be skipped without a reported count; there is no vision-based field understanding or guided first-use board login.
- Answer derivation is limited, and draft checks do not cover “Never mention” or claims beyond years.
- Only Ollama uses response schemas as sampling constraints; other providers rely on validation after decoding.
- Education/certifications are deliberately absent until a recorded form demonstrates a need.
  Skill durations are self-reported, never inferred; salary questions remain for the user and salary floors belong to saved searches.
- Upgraded profiles may need details entered or a resume re-imported after the old prose fields were removed.
- Jobs store source IDs as text rather than separate `JobSource` records; prompt guidance is deliberately a setting, not a table.
- Keyed job-source adapters are not built; credential selection among multiple labels for one source is unresolved.
  Google discovery is declined (ADR-0020); Brave is deferred.
- Regional remote titles can be narrower in the stored reading than intended when a source lists one city (for example “EMEA Remote” listed as Paris).
  Changing that interpretation remains open under ADR-0026/ADR-0023.
- Email notifications, third-party plugin/tool SDKs, in-app updates and unattended browser preparation remain deferred.
  Idle daemon auto-exit is not implemented.

## Next

1. Prepare the first pre-release under ADR-0029: refresh starter companies, align release versions, make the repository public as agreed, and run the tag-triggered draft-release workflow.
   The arm64 Linux runner requires a public repository or paid runner.
2. Verify downloaded installers: macOS quarantine/sidecars/Login Items attribution, Windows install/upgrade/uninstall hooks, and Linux packages.
   Measure real schedules, notification attribution, daemon upgrades and keystore behavior on Linux/Windows before claiming cross-platform verification.
3. Address the documented browser/answer/draft-check gaps and remaining settings requirements within their specifications.
4. Build a keyed source (Adzuna first) once a key is available and source credential selection is defined.
5. Resolve regional-title versus single-location eligibility before changing the reading across sources.
