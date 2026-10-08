# JobPilot CLI Commands

This reference covers the implemented command groups.
The CLI connects to the local daemon and starts it when needed; `--version` works without the daemon.
Use `jobpilot-cli <command> --help` for command-specific help.
IDs in the examples are placeholders; use the IDs printed by the corresponding list or create command.

## General

- `jobpilot-cli profiles list`: Lists local profiles and marks the installation's active profile.
- `jobpilot-cli profiles add <name>`: Creates a new empty local profile.
- `jobpilot-cli profiles use <id>`: Makes a profile active for desktop and unscoped CLI requests.
- `jobpilot-cli profiles rename <id> <name>`: Renames a local profile.
- `jobpilot-cli --workspace <id> <command>`: Runs one command against that profile without changing the active profile.
- `jobpilot-cli --version`: Prints the CLI's version, without starting the daemon.
  A CLI older than the running daemon uses it, and says on stderr that it is older; a newer one stops the daemon and starts its own (ADR-0029 §4).
- `jobpilot --version` and `jobpilot stop`: The daemon's version, and stopping the running daemon, which waits for its process to exit.
- `jobpilot alarms remove`: Removes every scheduled scout's alarm from the operating system's scheduler, which the Windows uninstaller runs after `jobpilot stop`.
  It refuses while a daemon is running, since the daemon would schedule its scouts again.
- `jobpilot alarms restore`: Schedules every enabled, scheduled scout again from the database, which the Windows installer runs once its files are in place.
  It does nothing when there is no database, or while a daemon is running.
- `jobpilot-cli onboard`: Initialize a new JobPilot installation.
  Sets up the data directory, database schema, verifies LLM connectivity, creates the initial candidate profile, optionally imports a resume, and configures your first job search (required — discovery cannot run without one).

## Candidate Profile

- `jobpilot-cli profile get`: Displays the current candidate profile information.
- `jobpilot-cli profile delete`: Deletes your profile, resumes, match analyses and applications.
  Jobs, saved searches, scouts and settings stay.
  Shows what goes and asks first; `--yes` skips the question.
- `jobpilot-cli profile update`: Interactively update the current candidate profile.
  Allows updating First Name, Last Name, Contact, Location, Summary, Skills and remote preference.
- `jobpilot-cli profile about`: Asks each About you question in turn — what you are looking for, what you value, the work you are proudest of, what colleagues come to you for, your career story, anything never to mention — and how you want letters to sound.
  Enter keeps an answer; clearing one removes it; nothing is required.
  Cover letters and written answers are built from what you say here (`docs/02_features/16_about_you.md`).
- `jobpilot-cli profile about show`: Shows your answers, writing preferences, writing sample and kept answers.
- `jobpilot-cli profile about set [key] [answer]`: Sets or clears one answer by its key, or `writing_sample` / `writing_preferences`.
  `--file` reads it from a file, which suits a writing sample.
- `jobpilot-cli profile about forget [id]`: Forgets an answer you kept for similar questions.

## Matching and Applications

- `jobpilot-cli match analyze <job-id>`: Analyze fit against the current profile and print the score, confidence, rationale, strengths and gaps.
- `jobpilot-cli match list`: List stored match analyses.
- `jobpilot-cli application create <job-id>`: Start tracking an application; `--resume <resume-id>` associates a resume and `--mode assist|review` chooses the mode (default `review`).
- `jobpilot-cli application list`: List applications for the current profile.
- `jobpilot-cli application show <application-id>`: Show the application, its content and allowed next states.
- `jobpilot-cli application transition <application-id> <state>`: Request an allowed state transition; `--reason` records a note.
- `jobpilot-cli application prepare-in-browser <application-id>`: Fill known answers in the dedicated Chrome profile and leave the browser open for your review and submission.
  Assist applications cannot use this command.
  JobPilot never clicks Submit; inspect the application and run trail if preparation fails.
- `jobpilot-cli application submitted <application-id>`: Record that you submitted it yourself after the application is `APPROVED`; `--reason` records a note.
- `jobpilot-cli application agent-runs <application-id>`: Show browser preparation runs for this application.

`application show` reports the daemon's allowed transitions.
Every outcome after `SUBMITTED` is recorded by you; JobPilot does not infer an employer's response.

## Cover Letters and Answers

- `jobpilot-cli application cover-letter generate [application-id]`: Asks a few questions about this job first — what draws you to it, and up to two the model proposes from the posting — then writes the letter from your answers, your About you and your profile.
  Enter skips a question; `--no-questions` skips them all.
  Rewriting offers the same questions, prefilled.
- `jobpilot-cli application cover-letter list <application-id>`: Show every version and its review flags.
- `jobpilot-cli application cover-letter edit <cover-letter-id> "<content>"`: Replace a version's text.
- `jobpilot-cli application question add <application-id> "<label>"`: Add a question; `--type factual|narrative` defaults to `factual` and `--required` marks it required.
- `jobpilot-cli application question resolve <question-id>`: Resolve through the existing answer chain, leaving unsupported facts for you.
- `jobpilot-cli application question answer <question-id> "<answer>"`: Supply your answer.
- `jobpilot-cli application question list <application-id>`: Show questions, answers and resolution status.
- `jobpilot-cli application question keep [question-id]`: Keeps a written answer you wrote yourself, so written answers to similar questions draw on it.
- Every letter and written answer the CLI prints is followed by "To review" when there is something to look at: a model preamble, a stock phrase, a letter longer than you asked for, or a number of years you never gave (`docs/02_features/19_draft_checks.md`).
  Nothing is changed or blocked; you decide.

## Resume Management

- `jobpilot-cli resume import [path]`: Imports a resume file from the specified path, converts it to Markdown, and associates it with the current candidate profile.
  Review the model's proposed profile fields before applying them, as in onboarding.
  Missing suggestions default to your current values.
  If suggestion generation fails, the resume import itself still succeeds.
- `jobpilot-cli resume list`: Lists all resumes currently imported for the candidate.
- `jobpilot-cli resume delete [id]`: Deletes a resume.
  Applications that used it keep everything already written from it.
  Shows what goes and asks first; `--yes` skips the question.

## Job Discovery

- `jobpilot-cli search run`: Discovers jobs using your default saved search.
  Pass `--saved <name-or-id>` to run a different one, or give keywords for a one-off search.
  Discovery needs nonempty criteria; core applies the complete criteria before storing results.
  Requires internet access.
  Results include the new-job count and any source failures.
  When no source can be reached it says so and exits with an error; jobs already found stay available through `search list`.
- `jobpilot-cli search saved list`: Lists your saved searches; `*` marks the default.
- `jobpilot-cli search saved add [name]`: Saves a named search.
  Criteria flags: `--keywords`, `--title`, `--location` (words the job's location must contain), `--remote`, `--employment`, `--salary-min`, `--salary-max`, `--exclude-company` (repeatable), `--work-from` (the country you work from, by name or code: keeps jobs open there or anywhere, and jobs that do not say), `--include-relocation` (with `--work-from`, also keeps jobs elsewhere that offer or mention relocation or visa sponsorship), `--fit-hours` (leaves out jobs whose required working hours your profile's time zone cannot work) and `--hours-shift` (with `--fit-hours`, how many hours, 0 to 12, you would move your day for a job).
  The first search you save becomes the default.
- `jobpilot-cli search saved edit [name-or-id]`: Replaces a saved search's criteria; `--name` renames it.
- `jobpilot-cli search saved delete [name-or-id]`: Deletes a saved search.
  Deleting the default promotes another.
- `jobpilot-cli search saved default [name-or-id]`: Chooses the search `search run` uses by default.
- `jobpilot-cli search list`: Lists jobs already discovered, most recent first, from local history.
  Reads stored data only and contacts no job source, so it works offline.
  Jobs marked not interested are left out.
  Flags: `--source`, `--remote` (`remote`, `hybrid` or `onsite`), `--posted-within-days`, `--keywords` (only jobs whose title or company matches), `--limit` (default 50, maximum 200), `--offset` (skip this many; the output names the offset for the next page), `--not-interested` (only the jobs marked not interested, most recently first), and `--work-from` and `--include-relocation`, as for a saved search.
- `jobpilot-cli search templates list` and `search templates import [id...]`: Show the ready-made searches, and adopt some as saved searches.
  Each adopted search starts with your profile's country as the one you work from; `--work-from` chooses another.
- `jobpilot-cli job not-interested [id]`: Rules a job out.
  It leaves your lists and search results, and scouts pass it over; its match and applications are unchanged.
  `job interested [id]` undoes it.
  Neither asks, since both are reversible.
- `jobpilot-cli job delete [id]`: Deletes a job with its match and applications.
  A later search may store it again; to keep it away, mark it not interested instead.
- `jobpilot-cli search show [id]`: Shows one stored job in full, including its description, where it can be done from, what it says about relocation and visa sponsorship, and the working hours it requires or prefers, with the sentence each was read from.
  Reads local history only.
- `jobpilot-cli company watch [address]`: Watches a company, from the address of its Greenhouse, Lever, Ashby, Workable or SmartRecruiters hiring page or of any posting on it.
  Checks and names the board; subsequent discovery searches its openings.
  Watching alone imports no jobs; see [Company watchlists](02_features/17_company_watchlists.md).
- `jobpilot-cli company list`: Lists the companies you watch, with when each page was last checked and how that went: its open roles, or that it was gone or could not be reached.
- `jobpilot-cli company rename [id] [name]` and `company unwatch [id]`: Rename a watched company, or stop watching it.
  Jobs already found from it stay.
- `jobpilot-cli company suggestions`: Lists the starter list's companies you do not watch yet, each with how many of its open roles could be done from anywhere or from more than one continent when measured, and the address to watch it by.
- `jobpilot-cli search import [url]`: Imports one job by its URL.
  When no source's API can fetch that URL, the page is opened in the browser worker and its content extracted (see `docs/02_features/04_job_discovery.md` AC-007).
  Requires internet access.

## Scouts

A scout runs a saved search, scores what it finds, and creates applications for you to review.
It never contacts an employer and never submits (`docs/02_features/10_scouts.md`).

- `jobpilot-cli scout list`: Lists your scouts with their rules and when each runs: its next run, whether it is running or waiting for another scout's run, and anything stopping its schedule, such as a schedule switched off in macOS's Login Items.
- `jobpilot-cli scout add [name] --search <saved-search-id>`: Creates a scout.
  `--min-score` (default 70), `--max-per-run` (default 10) and `--source` (`local` to consider stored jobs, `sources` to fetch from the job boards first).
  `--every <hours>` (1 to 24) or `--daily <HH:MM>` schedules it; without either it runs only when you run it.
- `jobpilot-cli scout schedule [scout-id]`: Sets when a scout runs on its own: `--every <hours>`, `--daily <HH:MM>` on this computer's clock, or `--off`.
  The operating system's scheduler starts it, whether or not JobPilot is open, while you are logged in and the computer is awake; a run missed while the computer was off or asleep runs once when it is next on (ADR-0027).
- `jobpilot-cli scout run [scout-id]`: Runs a scout now and prints what it considered, rejected and created.
  Scout runs happen one at a time, so this waits for one already in progress.
- `jobpilot-cli scout runs`: Lists recent scout runs, each marked as started by you or on schedule.
- `jobpilot-cli scout trail [run-id]`: Shows each step one run took.
- `jobpilot-cli scout delete [scout-id]`: Deletes a scout and its schedule.
  Applications it created stay.
- `jobpilot-cli notifications`: Shows what your scouts found while you were away, or that one could not run, and marks it seen.
  `--all` shows every notification and marks nothing (`docs/02_features/11_notifications.md`).

## Activity and Credentials

- `jobpilot-cli activity list`: List agent runs; narrow with `--kind prepare_application|scout_run` or `--status running|completed|blocked|failed` and page with `--limit` (default 50, maximum 200) and `--offset`.
- `jobpilot-cli activity show <run-id>`: Show a run's steps and policy results.
- `jobpilot-cli credentials list`: List configured credential references without showing values.
- `jobpilot-cli credentials set <source> --label <label>`: Store or replace a key in the OS keychain using a masked prompt or piped standard input.
- `jobpilot-cli credentials delete <source> --label <label>`: Remove that stored key.

Labels default to `default`.
For a hosted model use source `llm`, then select the label in `settings set-llm`.
Storing a job-source key does not enable an unimplemented connector; current discovery sources need no keys.

## Deleting Data

The destructive profile, resume, job, application, cover-letter and data commands show their impact and ask for confirmation.
`--yes` skips that question, but still shows the impact.
Without `--yes`, a noninteractive invocation does not delete anything.
If the impact changes before deletion, JobPilot shows the new list and asks again.
Saved-search, scout, watched-company and credential removal commands do not use this confirmation flow.

- `jobpilot-cli application delete [id]`: Deletes an application with its answers, cover letter versions, status history and browser runs.
  Nothing already sent to an employer is withdrawn.
- `jobpilot-cli application cover-letter delete [id]`: Deletes an earlier cover letter version.
  The version an application uses cannot be deleted while it is in use.
- `jobpilot-cli data clear activity`: Deletes every agent run and its steps.
- `jobpilot-cli data clear jobs`: Deletes every stored job no application uses, keeping those marked not interested so they are not offered again.
- `jobpilot-cli data delete-all`: Resets the selected profile — its database contents, API keys in the system keychain, browser sign-ins and Scout alarms — while keeping other profiles and device preferences.
  Asks for the word `delete`.
  An API key supplied through an environment variable cannot be unset by JobPilot, and the confirmation says so.

## Settings

- `jobpilot-cli settings show`: Displays the configured LLM provider, base URL, and model (or the default if not yet configured), the API key's label and whether it is stored, and, for a model that is not on this computer, where the candidate's data is sent.
- `jobpilot-cli settings prompts`: Lists the prompts you can give guidance to — matching, cover letters and written answers — and whether each follows its default or your own guidance (ADR-0031).
  The rules that keep JobPilot to what you have told it are not guidance and cannot be changed.
- `jobpilot-cli settings prompts show <key>`: Shows a prompt's guidance, and its default beside your own.
  For cover letters, also says that length and tone are your writing preferences.
- `jobpilot-cli settings prompts edit <key>`: Replaces a prompt's guidance with your own, edited in `$VISUAL` or `$EDITOR` starting from what it is now.
  `--file <path>` reads it from a file, or `-` from standard input.
  Empty guidance, or more than 2,000 characters, is refused.
- `jobpilot-cli settings prompts reset <key>`: Puts a prompt back on its default, which it then follows as later releases change it.
- `jobpilot-cli settings notifications`: Shows whether JobPilot posts notifications about scout runs, and the score a strong match needs.
  `--on` / `--off` turn posting on or off; turned off, notifications still wait on the Dashboard and in `notifications`.
  `--strong-match <1-100>` sets the score.
- `jobpilot-cli settings set-llm`: Interactively configure the LLM provider and model.
  Choose from known providers (Ollama, llama.cpp, LM Studio) or a custom OpenAI-compatible endpoint, confirm/edit the base URL, and enter the model name manually.
  For a custom endpoint — a hosted model such as OpenRouter, OpenAI or Groq — enter the base URL as the provider gives it, with its version, then choose an API key: none, one already stored for the model, or a new one, typed without echo and stored in the keychain (ADR-0030).
  Settings are saved even if the endpoint can't be reached at save time.

## Usage Example

```bash
# First time setup
jobpilot-cli onboard

# Import a resume
jobpilot-cli resume import ~/Documents/resume.pdf

# View profile
jobpilot-cli profile get

# Update profile
jobpilot-cli profile update

# Configure the LLM provider/model
jobpilot-cli settings set-llm

# Save what you are looking for, then discover against it
jobpilot-cli search saved add "Backend Go roles" --keywords "golang backend" --remote remote
jobpilot-cli search run
jobpilot-cli search saved add "Remote from Lagos" --title "backend engineer" --work-from Nigeria --include-relocation
jobpilot-cli search list --keywords golang --limit 20
jobpilot-cli search list --limit 20 --offset 20
```

```bash
# After preparing and reviewing the application
jobpilot-cli application transition <application-id> APPROVED

# After you submit it yourself in the browser
jobpilot-cli application submitted <application-id>

# Record an interview, then an offer, when each happens
jobpilot-cli application transition <application-id> INTERVIEWING
jobpilot-cli application transition <application-id> OFFERED
```
