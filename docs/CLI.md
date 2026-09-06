# JobPilot CLI Commands

This document lists the currently implemented CLI commands for the JobPilot.

## General
- `jobpilot-cli onboard`: Initialize a new JobPilot installation. Sets up the data directory, database schema, verifies LLM connectivity, and creates the initial candidate profile.

## Candidate Profile
- `jobpilot-cli profile get`: Displays the current candidate profile information.
- `jobpilot-cli profile update`: Interactively update the current candidate profile. Allows updating Name, Contact, Location, Summary, Skills, Salary Preferences, Work Authorization, and Application Preferences.

## Resume Management
- `jobpilot-cli resume import [path]`: Imports a resume file from the specified path, converts it to Markdown, and associates it with the current candidate profile. Afterward, the LLM proposes a Summary and Skills draft from the resume; you can review and edit each field before it's applied to your profile. This same review step also runs during `onboard` when a resume is imported there. If suggestion generation fails, the resume import itself still succeeds.
- `jobpilot-cli resume list`: Lists all resumes currently imported for the candidate.
- `jobpilot-cli resume delete [id]`: Deletes a specific resume by its ID.

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
```
