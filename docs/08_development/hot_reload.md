# Hot Reload

`make dev` runs the daemon under [air](https://github.com/air-verse/air), which rebuilds and restarts it whenever its Go code or migrations change.
The desktop has its own hot reload, Vite's, so a working session is two terminals:

```sh
make dev          # the daemon, rebuilt on change
make run-desktop  # the desktop, which finds that daemon
```

Start `make dev` first.
The desktop and CLI start a daemon of their own when none is running, and a daemon refuses to start while another answers, so whichever came first is the one in use.
If `make dev` reports another daemon running, stop that one (its PID is in the message) and run it again.

## What is watched

`.air.toml` watches `cmd/jobpilot` and `internal`, for `.go` and `.sql` files, leaving out tests.
The SQL is the migrations, which are embedded in the binary, so changing one needs a rebuild as much as changing Go does: the restarted daemon applies it on start.
The CLI is not rebuilt; `make build` builds it.

The binary goes to `bin/dev/`, which is ignored.
A build that fails leaves the running daemon up, and its errors are in air's output and `bin/dev/build-errors.log`.
air is pinned by `AIR_VERSION` in the Makefile and installed into `./bin` on first use, as golangci-lint is.

## How the clients follow a restart

Each start of the daemon listens on a free port and records it in the discovery file (ADR-0012), so a restarted daemon is usually on a different port.
air interrupts the old daemon rather than killing it, so it shuts down as it would in use: it removes its discovery file, closes the database and closes its browser.

- The CLI reads the discovery file on every command, so it finds the new daemon without doing anything.
- The desktop notices when a request cannot reach the daemon, and waits up to 15 seconds for one to answer (`find_daemon` in `desktop/src-tauri/src/daemon.rs`).
  Found, it carries on where it was and refetches what is on screen; the cache is kept, since a daemon found through the discovery file serves the same data directory.
  It never starts a daemon while it waits, because one started in the gap between the old daemon stopping and the new one starting would be a second on the same database.
  If none comes back, it says it could not connect, and Retry starts one.

Anything in progress when the daemon restarts fails and says so: a discovery run, an LLM call, a browser window being filled.
The browser window closes with the daemon that opened it.
