# Product Requirements

## MVP requirements

### PR-001 Candidate profile

The user can create and maintain a local candidate profile containing structured professional information.

### PR-002 Resume management

The user can import and manage one or more resumes locally.

### PR-003 Job discovery

The user can configure keywords, locations, job types, sources, and other search preferences and run job discovery against supported sources.

### PR-004 Custom job URLs

The user can provide custom job URLs for analysis.

### PR-005 Job normalization

Jobs from different sources are normalized into a common schema.

### PR-006 Deduplication

The system detects duplicate jobs across sources.

### PR-007 Matching

The system evaluates jobs against the candidate profile and resume and produces a match score and structured explanation.

### PR-008 Application answer generation

The system generates tailored answers for application questions using the job description and candidate context.

### PR-009 Cover letters

The system can generate a tailored cover letter for a job.

### PR-010 Local history

Jobs, analyses, generated application content, and application history are stored locally.

### PR-011 LLM abstraction

The application accesses the selected LLM through a provider abstraction.

### PR-012 Local LLM

Ollama is supported as the initial local LLM provider.

### PR-013 Prompt customization

Users can customize the guidance for matching, cover letters and written answers without editing source code.
The safety rules and response contracts remain code-owned ([ADR-0031](../09_decisions/0031-what-a-user-may-change-in-a-prompt.md)).

### PR-014 CLI

The application exposes practical commands for setup, profile management, searching, analysis, history, and diagnostics.

### PR-015 Desktop UI

The application exposes equivalent core user workflows through a native-feeling desktop interface.

### PR-016 Offline history

Previously stored jobs, analyses, generated content, and history remain accessible without internet.

## Post-MVP requirements

This is the original scope boundary, not a list of unimplemented features.
Browser preparation, Review mode, Scouts, desktop notifications, Activity, application memory, OS scheduling and packaging are now built.
Published installers and cross-platform runtime verification remain outstanding.
See [current state](current_state.md) for status and [roadmap](roadmap.md) for phases.

- Browser automation for application forms.
- Review workflow with browser form filling.
- Scouts: unattended discovery and matching that prepares applications for review.
- More job source connectors.
- Authenticated website sessions.
- Connector SDK.
- Tool/plugin SDK.
- Email notifications.
- Agent trace viewer.
- Advanced policy engine.
- Application memory.
- Cross-platform startup/scheduling integration.
- Application packaging/update mechanism.

## Product constraints

- No mandatory user account.
- No mandatory hosted backend.
- Local-first storage.
- User controls consequential external actions.
- JobPilot never submits an application; the user submits in the browser.
- No assumption that arbitrary websites permit automation.
- No guarantee that every job site can be automated.
