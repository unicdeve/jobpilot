# Job Agent CLI Commands

This document lists the currently implemented CLI commands for the Job Agent.

## General
- `job-agent-cli onboard`: Initialize a new Job Agent installation. Sets up the data directory, database schema, verifies LLM connectivity, and creates the initial candidate profile.

## Candidate Profile
- `job-agent-cli profile get`: Displays the current candidate profile information.
- `job-agent-cli profile update`: Interactively update the current candidate profile. Allows updating Name, Contact, Location, Summary, Skills, Salary Preferences, Work Authorization, and Application Preferences.

## Resume Management
- `job-agent-cli resume import [path]`: Imports a resume file from the specified path, converts it to Markdown, and associates it with the current candidate profile.
- `job-agent-cli resume list`: Lists all resumes currently imported for the candidate.
- `job-agent-cli resume delete [id]`: Deletes a specific resume by its ID.

## Usage Example
```bash
# First time setup
job-agent-cli onboard

# Import a resume
job-agent-cli resume import ~/Documents/resume.pdf

# View profile
job-agent-cli profile get

# Update profile
job-agent-cli profile update
```
