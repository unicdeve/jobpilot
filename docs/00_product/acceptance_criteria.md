# Global Acceptance Criteria

## Installation

### AC-G-001
The application can be installed on macOS, Linux, and Windows through published release artifacts.

### AC-G-002
A user can use the CLI without launching the desktop UI.

### AC-G-003
A user can use the desktop UI without requiring a hosted account.

## Privacy

### AC-G-004
Candidate data is stored locally by default.

### AC-G-005
No telemetry is transmitted in MVP.

### AC-G-006
Secrets are not stored as plaintext application records.

## LLM

### AC-G-007
The domain layer depends on an LLM abstraction, not directly on Ollama.

### AC-G-008
Malformed LLM output cannot directly mutate domain state.

### AC-G-009
The system does not fabricate candidate facts.

## Offline

### AC-G-010
Stored jobs/history/profile remain accessible offline.

### AC-G-011
Fresh discovery clearly reports that internet access is required when offline.

## Job discovery

### AC-G-012
Search criteria can be saved and reused.

### AC-G-013
Source failures are isolated from successful sources.

### AC-G-014
Duplicate jobs are deduplicated.

## Matching

### AC-G-015
Match scores are constrained to 0-100.

### AC-G-016
Match explanations contain structured strengths/gaps/concerns.

## Applications

### AC-G-017
Review is the default mode.

### AC-G-018
Review mode cannot submit without explicit user approval.

### AC-G-019
JobPilot never submits an application to an external system, in any mode, under any configuration.

### AC-G-020
Unknown factual application questions are not guessed.

## Data lifecycle

### AC-G-021
Users can delete local application data.

### AC-G-022
Deletion handles derived data consistently.

## Security

### AC-G-023
External webpages are treated as untrusted input.

### AC-G-024
LLM output cannot escalate tool permissions.

### AC-G-025
Browser automation stops safely on CAPTCHA or ambiguous required fields.
