# JobPilot CLI Commands

This document lists the currently implemented CLI commands for the JobPilot.

## General
- `jobpilot-cli onboard`: Initialize a new JobPilot installation. Sets up the data directory, database schema, verifies LLM connectivity, creates the initial candidate profile, optionally imports a resume, and configures your first job search (required — discovery cannot run without one).

## Candidate Profile
- `jobpilot-cli profile get`: Displays the current candidate profile information.
- `jobpilot-cli profile delete`: Deletes your profile, resumes, match analyses and applications. Jobs, saved searches, scouts and settings stay. Shows what goes and asks first; `--yes` skips the question.
- `jobpilot-cli profile update`: Interactively update the current candidate profile. Allows updating First Name, Last Name, Contact, Location, Summary, Skills and remote preference.
- `jobpilot-cli profile about`: Asks each About you question in turn — what you are looking for, what you value, the work you are proudest of, what colleagues come to you for, your career story, anything never to mention — and how you want letters to sound. Enter keeps an answer; clearing one removes it; nothing is required. Cover letters and written answers are built from what you say here (`docs/02_features/16_about_you.md`).
- `jobpilot-cli profile about show`: Shows your answers, writing preferences, writing sample and kept answers.
- `jobpilot-cli profile about set [key] [answer]`: Sets or clears one answer by its key, or `writing_sample` / `writing_preferences`. `--file` reads it from a file, which suits a writing sample.
- `jobpilot-cli profile about forget [id]`: Forgets an answer you kept for similar questions.

## Cover Letters and Answers
- `jobpilot-cli application cover-letter generate [application-id]`: Asks a few questions about this job first — what draws you to it, and up to two the model proposes from the posting — then writes the letter from your answers, your About you and your profile. Enter skips a question; `--no-questions` skips them all. Rewriting offers the same questions, prefilled.
- `jobpilot-cli application question keep [question-id]`: Keeps a written answer you wrote yourself, so written answers to similar questions draw on it.

## Resume Management
- `jobpilot-cli resume import [path]`: Imports a resume file from the specified path, converts it to Markdown, and associates it with the current candidate profile. Afterward, the LLM proposes a draft of First Name, Last Name, Contact, Location, Summary, and Skills from the resume; you can review and edit each field (defaulting to your current profile value if the draft left it blank) before it's applied to your profile. This same review step also runs during `onboard` when a resume is imported there. If suggestion generation fails, the resume import itself still succeeds.
- `jobpilot-cli resume list`: Lists all resumes currently imported for the candidate.
- `jobpilot-cli resume delete [id]`: Deletes a resume. Applications that used it keep everything already written from it. Shows what goes and asks first; `--yes` skips the question.

## Job Discovery
- `jobpilot-cli search run`: Discovers jobs using your default saved search. Pass `--saved <name-or-id>` to run a different one, or give keywords for a one-off search. Discovery always needs criteria — the public job boards do no filtering of their own, so a search with nothing set would import every posting on every board. Requires internet access. It reports how many of the jobs it found are new, and names any source that could not be reached or answered with an error, since results without that source may be incomplete. When no source can be reached it says so and exits with an error; jobs already found stay available through `search list`.
- `jobpilot-cli search saved list`: Lists your saved searches; `*` marks the default.
- `jobpilot-cli search saved add [name]`: Saves a named search. Criteria flags: `--keywords`, `--title`, `--location` (words the job's location must contain), `--remote`, `--employment`, `--salary-min`, `--salary-max`, `--exclude-company` (repeatable), `--work-from` (the country you work from, by name or code: keeps jobs open there or anywhere, and jobs that do not say), `--include-relocation` (with `--work-from`, also keeps jobs elsewhere that offer or mention relocation or visa sponsorship), `--fit-hours` (leaves out jobs whose required working hours your profile's time zone cannot work) and `--hours-shift` (with `--fit-hours`, how many hours, 0 to 12, you would move your day for a job). The first search you save becomes the default.
- `jobpilot-cli search saved edit [name-or-id]`: Replaces a saved search's criteria; `--name` renames it.
- `jobpilot-cli search saved delete [name-or-id]`: Deletes a saved search. Deleting the default promotes another.
- `jobpilot-cli search saved default [name-or-id]`: Chooses the search `search run` uses by default.
- `jobpilot-cli search list`: Lists jobs already discovered, most recent first, from local history. Reads stored data only and contacts no job source, so it works offline. Jobs marked not interested are left out. Flags: `--keywords` (only jobs whose title or company matches), `--limit` (default 50, maximum 200), `--offset` (skip this many; the output names the offset for the next page), `--not-interested` (only the jobs marked not interested, most recently first), and `--work-from` and `--include-relocation`, as for a saved search.
- `jobpilot-cli search templates list` and `search templates import [id...]`: Show the ready-made searches, and adopt some as saved searches. Each adopted search starts with your profile's country as the one you work from; `--work-from` chooses another.
- `jobpilot-cli job not-interested [id]`: Rules a job out. It leaves your lists and search results, and scouts pass it over; its match and applications are unchanged. `job interested [id]` undoes it. Neither asks, since both are reversible.
- `jobpilot-cli job delete [id]`: Deletes a job with its match and applications. A later search may store it again; to keep it away, mark it not interested instead.
- `jobpilot-cli search show [id]`: Shows one stored job in full, including its description, where it can be done from, what it says about relocation and visa sponsorship, and the working hours it requires or prefers, with the sentence each was read from. Reads local history only.
- `jobpilot-cli company watch [address]`: Watches a company, from the address of its Greenhouse, Lever or Ashby hiring page or of any posting on it. Its board is read once to check it, and the company is named from it. From then on your saved searches find its openings like any source's; watching imports nothing by itself (`docs/02_features/17_company_watchlists.md`).
- `jobpilot-cli company list`: Lists the companies you watch, with when each page was last checked and how that went: its open roles, or that it was gone or could not be reached.
- `jobpilot-cli company rename [id] [name]` and `company unwatch [id]`: Rename a watched company, or stop watching it. Jobs already found from it stay.
- `jobpilot-cli company suggestions`: Lists the starter list's companies you do not watch yet, each with how many of its open roles could be done from anywhere or from more than one continent when measured, and the address to watch it by.
- `jobpilot-cli search import [url]`: Imports one job by its URL. When no source's API can fetch that URL, the page is opened in the browser worker and its content extracted (see `docs/02_features/04_job_discovery.md` AC-007). Requires internet access.

## Deleting Data

Every deletion first shows what it takes with it and asks. `--yes` skips the question but not the list. Without a terminal to ask on, nothing is deleted. If what it reaches changes before it is carried out — a scout created an application meanwhile — the new list is shown and asked about again.

- `jobpilot-cli application delete [id]`: Deletes an application with its answers, cover letter versions, status history and browser runs. Nothing already sent to an employer is withdrawn.
- `jobpilot-cli application cover-letter delete [id]`: Deletes an earlier cover letter version. The version an application uses cannot be deleted while it is in use.
- `jobpilot-cli data clear activity`: Deletes every agent run and its steps.
- `jobpilot-cli data clear jobs`: Deletes every stored job no application uses, keeping those marked not interested so they are not offered again.
- `jobpilot-cli data delete-all`: Deletes everything JobPilot stores — the database's contents, API keys in the system keychain, the browser profile and its job board sign-ins, and the daemon's log — leaving it as a fresh install. Asks for the word `delete`. An API key supplied through an environment variable cannot be unset by JobPilot, and the confirmation says so.

## Settings
- `jobpilot-cli settings show`: Displays the configured LLM provider, base URL, and model (or the default if not yet configured).
- `jobpilot-cli settings set-llm`: Interactively configure the LLM provider and model. Choose from known providers (Ollama, llama.cpp, LM Studio) or a custom OpenAI-compatible endpoint, confirm/edit the base URL, and enter the model name manually. Settings are saved even if the endpoint can't be reached at save time.

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
# After JobPilot fills the form in the browser and you send it yourself
jobpilot-cli application submitted <application-id>

# Then record what the employer said
jobpilot-cli application transition <application-id> INTERVIEWING
jobpilot-cli application transition <application-id> OFFERED
jobpilot-cli application transition <application-id> NO_RESPONSE
```

`application show` lists the states an application can move to next, as the daemon's own state machine reports them.
No outcome is ever inferred: every state after SUBMITTED is something the user tells JobPilot.

The rest of the `match` and `application` command groups are implemented but not yet documented here.
