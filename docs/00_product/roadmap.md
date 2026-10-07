# Roadmap

This is the phase scope, not an ordered list of unfinished work.
Phases 0–8 are substantially implemented; Phase 9 remains deferred, and Phase 10 is built but awaits the first release and installed-build validation.
See [current state](current_state.md) for gaps, platform verification and the next tasks.

## Phase 0 - Project foundation

- Repository
- Documentation
- Go module
- Configuration
- SQLite
- Logging
- CLI skeleton
- Test infrastructure

## Phase 1 - Candidate profile and documents

- Candidate schema
- Profile management
- Resume import
- Markdown conversion
- Resume history
- Local storage/deletion

## Phase 2 - Job discovery

- Search criteria
- Job schema
- Source interface
- Initial public connectors
- Custom URLs
- Normalization
- Deduplication
- Search history

## Phase 3 - Matching

- LLM abstraction
- Ollama provider
- Structured matching
- Match score
- Match explanation
- Confidence handling
- Prompt customization

## Phase 4 - Application generation

- Application schema
- Cover letters
- Application answers
- Application memory/history
- Reviewable generated content

## Phase 5 - Desktop

- Tauri application
- Native-feeling navigation
- Jobs
- Analyzed jobs (matches within Jobs)
- Applications
- Profile
- Settings
- Agent activity

## Phase 6 - Browser automation

- Go Playwright worker
- Browser lifecycle
- Local browser profile
- Form extraction
- Deterministic field filling
- Generated answer filling

## Phase 7 - Review mode

- Application review state
- Approval UI
- Browser handoff
- User-reported external submission and outcome tracking

## Phase 8 - Scouts

- Scout configuration over a saved search
- Scout runs: obtain, match, create applications for review
- Eligibility rules (match score, blocked companies, salary floor, per-run ceiling)
- Rule evaluation, deterministic and in core
- Run history in the agent trail
- Notifications

## Phase 9 - Extensibility

- Connector SDK
- Tool SDK
- Plugin protocol
- Plugin permissions
- Example community extensions

## Phase 10 - Distribution

- Cross-platform installers
- OS startup integration
- Update strategy
- Release automation
