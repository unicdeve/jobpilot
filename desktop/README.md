# JobPilot desktop

The desktop app uses Tauri 2, React and TypeScript.
It shares the core daemon with the CLI over loopback HTTP ([ADR-0012](../docs/09_decisions/0012-loopback-http-ipc.md)) and starts a daemon when none is running.

## Prerequisites

Install Go at the version required by [go.mod](../go.mod) or later, Bun, rustup and Tauri's platform build prerequisites.
The Rust toolchain is pinned in [rust-toolchain.toml](src-tauri/rust-toolchain.toml).
Install Google Chrome for fallback job import; the daemon downloads the Playwright driver on first browser use.
Choose a local model server such as Ollama, or configure a hosted provider during first-run setup.

## Running it

From the repository root, install the pinned frontend dependencies:

```sh
make desktop-deps
```

For hot reload, start the daemon first, then the desktop in another terminal:

```sh
make dev
make run-desktop
```

See [hot reload](../docs/08_development/hot_reload.md) for restart behavior.
To let the desktop start a built daemon instead:

```sh
make build
JOBPILOT_DAEMON_PATH="$PWD/bin/jobpilot" make run-desktop
```

On Windows, set `JOBPILOT_DAEMON_PATH` to the absolute path of `bin/jobpilot.exe` in your shell before running `make run-desktop`.
The daemon binary lookup order is beside the desktop executable, on `PATH`, then `JOBPILOT_DAEMON_PATH`.
An already-running daemon is discovered before a binary is needed.
`make run-desktop` runs `bun tauri dev`; it does not build the daemon or install frontend dependencies.

## Checks and formatting

From `desktop/`:

```sh
bun run check                        # Biome formatting, lint and imports; no writes
bun run fix                          # safe Biome fixes
bun run build                        # TypeScript check and production frontend build
cargo clippy --manifest-path src-tauri/Cargo.toml --locked -- -D warnings
cargo test --manifest-path src-tauri/Cargo.toml --locked
```

From the repository root, `make fmt` formats Go and frontend files, and `make lint-fix` applies their automatic lint fixes.
`make check` includes the Go checks and `bun run check`; run the frontend build and Rust checks separately.
See [testing](../docs/08_development/testing.md) for the complete validation workflow.

## Packaging

From the repository root, `make bundle` builds installers for the current machine and includes the daemon and CLI as sidecars.
Release artifacts have not yet been published; see [packaging](../docs/08_development/packaging.md) for target platforms, versioning and release validation.
The [desktop architecture](../docs/01_architecture/02_desktop.md) describes UI conventions, navigation and daemon communication.
