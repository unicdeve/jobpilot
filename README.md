# Job Agent

Open-source, local-first AI job search and application assistant.

Job Agent helps users discover relevant jobs, evaluate them against their
candidate profile and resume, generate tailored application answers, and prepare
applications for user review. It is designed to run locally, preserve user data
locally, and support local LLMs such as Ollama.

## Product principles

- Local-first and privacy-preserving by default
- Human-controlled external actions
- Review mode is the default application mode
- Deterministic rules take precedence over LLM guesses
- Internet is required for discovering new jobs
- Existing history remains usable offline
- Extensible through connectors, tools, and plugins
- Cross-platform: macOS, Linux, and Windows

## MVP

The MVP focuses on:

1. Maintaining a candidate profile and resume locally.
2. Searching supported job sources.
3. Normalizing and deduplicating jobs.
4. Matching jobs against the user's profile and resume.
5. Producing structured match explanations.
6. Generating tailored application answers and cover letters.
7. Persisting job and application history locally.
8. Providing CLI and desktop interfaces.

Browser-based form filling, automatic submission, extensive third-party
integrations, and the plugin ecosystem are post-MVP.

## Documentation

Start with `AGENTS.md`, then `.agents/SETUP.md` and `docs/MASTER_SPEC.md`. For a
list of available CLI commands, see [docs/CLI.md](docs/CLI.md).

## Development

The core runtime is Go. The desktop UI uses Tauri + React + TypeScript. SQLite
is the local database. Browser automation is planned around Go Playwright unless
implementation evidence shows a different approach is necessary.

See `docs/01_architecture/` for the architecture and `docs/09_decisions/` for
accepted decisions.
