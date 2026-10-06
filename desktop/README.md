# JobPilot desktop

The desktop app: Tauri 2, React and TypeScript.
It talks to the JobPilot daemon over loopback HTTP (ADR-0012) and starts the daemon if none is running.

## Running it

From the repository root:

```sh
make build        # the daemon and CLI, into bin/
cd desktop && bun install
JOBPILOT_DAEMON_PATH=../bin/jobpilot bun tauri dev
```

The app looks for the `jobpilot` daemon binary next to its own executable, then on `PATH`, then at `JOBPILOT_DAEMON_PATH`.
`make run-desktop` runs `bun tauri dev` and relies on the first two.

## Formatting and lint

[Biome](https://biomejs.dev) formats and lints the TypeScript, CSS and JSON, and orders imports, as `biome.json` says.

```sh
bun run check     # what CI and `make check` run; writes nothing
bun run fix       # formats, orders imports, and applies the safe lint fixes
```

From the repository root, `make fmt` and `make lint-fix` include the desktop, and `make check` fails on anything `bun run check` reports.
The TypeScript is type-checked by `bun run build`.

`docs/01_architecture/02_desktop.md` describes how the app is built.
