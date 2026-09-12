# JobPilot CLI Commands

This document lists the currently implemented CLI commands for the JobPilot.

## General
- `jobpilot-cli onboard`: Initialize a new JobPilot installation. Sets up the data directory, database schema, verifies LLM connectivity, and creates the initial candidate profile.

## Candidate Profile
- `jobpilot-cli profile get`: Displays the current candidate profile information.
- `jobpilot-cli profile update`: Interactively update the current candidate profile. Allows updating First Name, Last Name, Contact, Location, Summary, Skills, Salary Preferences, Work Authorization, and Application Preferences.

## Resume Management
- `jobpilot-cli resume import [path]`: Imports a resume file from the specified path, converts it to Markdown, and associates it with the current candidate profile. Afterward, the LLM proposes a draft of First Name, Last Name, Contact, Location, Summary, and Skills from the resume; you can review and edit each field (defaulting to your current profile value if the draft left it blank) before it's applied to your profile. This same review step also runs during `onboard` when a resume is imported there. If suggestion generation fails, the resume import itself still succeeds.
- `jobpilot-cli resume list`: Lists all resumes currently imported for the candidate.
- `jobpilot-cli resume delete [id]`: Deletes a specific resume by its ID.

## Job Discovery
- `jobpilot-cli search run [keywords]`: Searches every registered job source for new jobs matching the keywords, normalizes and deduplicates the results, and stores them. Requires internet access.
- `jobpilot-cli search list`: Lists jobs already discovered, most recent first, from local history. Reads stored data only and contacts no job source, so it works offline. Flags: `--keywords` (only jobs whose title or company matches), `--limit` (default 50, maximum 200), `--offset` (skip this many; the output names the offset for the next page).
- `jobpilot-cli search show [id]`: Shows one stored job in full, including its description. Reads local history only.
- `jobpilot-cli search import [url]`: Imports one job by its URL. When no source's API can fetch that URL, the page is opened in the browser worker and its content extracted (see `docs/02_features/04_job_discovery.md` AC-007). Requires internet access.

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

# Discover jobs, then browse what was stored (works offline)
jobpilot-cli search run golang
jobpilot-cli search list --keywords golang --limit 20
jobpilot-cli search list --limit 20 --offset 20
```

The `match` and `application` command groups are implemented but not yet
documented here.
