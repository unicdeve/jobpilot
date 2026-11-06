# Project Context

JobPilot is an open-source, local-first job search and application assistant.
The Go daemon owns local data and core rules; the CLI and Tauri desktop share its loopback API.
SQLite stores application data, OS credential storage holds keys, and Ollama is the default local model.
Hosted OpenAI-compatible models are optional and require disclosure of the data sent to their host.

The candidate writes and sends their own applications: JobPilot never writes a letter or an answer, and never fills in or submits a form (ADR-0034).
It opens the application page in the user's browser and keeps their details, letter and answers ready to copy.
Scouts perform unattended discovery and matching and create applications for the candidate, without contacting employers.
Existing history remains usable offline.

## Current focus

The core workflows, desktop and Scouts are implemented; browser preparation and generated application content were removed by ADR-0034.
Packaging and release automation are built; the next milestone is the first pre-release and downloaded-installer validation on macOS, Linux and Windows.
Plugin/tool SDKs remain deferred.

Use [current state](current_state.md) for capabilities, known gaps and verification status, and the [roadmap](roadmap.md) for phase scope.
Start implementation work from [MASTER_SPEC](../MASTER_SPEC.md), the relevant feature specification and accepted [ADRs](../09_decisions/).
Avoid distributed infrastructure or behavior unsupported by those requirements.
