# Project Context

JobPilot is an open-source, local-first job search and application assistant.
The Go daemon owns local data and core rules; the CLI and Tauri desktop share its loopback API.
SQLite stores application data, OS credential storage holds keys, and Ollama is the default local model.
Hosted OpenAI-compatible models are optional and require disclosure of the data sent to their host.

Review is the default application mode.
JobPilot fills supported browser forms but never submits them; the user reviews and submits in the open browser.
Scouts perform unattended discovery and matching and create applications for review, stopping before the browser.
Existing history remains usable offline.

## Current focus

The core workflows, desktop, browser preparation and Scouts are implemented.
Packaging and release automation are built; the next milestone is the first pre-release and downloaded-installer validation on macOS, Linux and Windows.
Plugin/tool SDKs remain deferred.

Use [current state](current_state.md) for capabilities, known gaps and verification status, and the [roadmap](roadmap.md) for phase scope.
Start implementation work from [MASTER_SPEC](../MASTER_SPEC.md), the relevant feature specification and accepted [ADRs](../09_decisions/).
Avoid distributed infrastructure or behavior unsupported by those requirements.
