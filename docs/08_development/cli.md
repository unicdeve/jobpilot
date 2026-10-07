# CLI Architecture

`jobpilot-cli` uses Cobra and is a client of the Go daemon, `jobpilot`.
It discovers or starts the daemon through the shared discovery-file protocol rather than opening SQLite directly ([ADR-0012](../09_decisions/0012-loopback-http-ipc.md)).
The daemon owns persistence, business rules, model calls and browser work.

Use the [CLI reference](../CLI.md) for implemented commands and examples.
`jobpilot-cli --help` and each subcommand's `--help` are the complete command/flag reference.
Avoid maintaining a second conceptual command list here.

## Behavior

- Commands share the desktop's core services and local data.
- Errors should name the failed action and return a nonzero exit status.
- Destructive data changes require the deletion preview/confirmation flow.
- Unknown factual answers remain for the user; the CLI never invents candidate information.
- Approval and submission tracking update local state; no CLI command submits externally.
- Long-running model and Scout work is cancellable and is not cut off by a fixed total client timeout.

See [IPC](../01_architecture/07_ipc.md), [hot reload](hot_reload.md) and [packaging](packaging.md) for daemon discovery, development and release behavior.
