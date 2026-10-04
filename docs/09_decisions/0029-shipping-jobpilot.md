# ADR-0029: Shipping JobPilot

## Status

Accepted on 2026-10-03.
Four choices were made that day: the bundle identifier is `io.github.unicdeve.jobpilot`, releases are unsigned until signing is paid for, updating is manual, and the repository is public when the first release goes out.
Decisions 1 to 10 say how Phase 10 of the roadmap is built around them, and were accepted with them.

It builds the post-MVP requirement "Application packaging/update mechanism" (`docs/00_product/01_requirements.md`), within `docs/08_development/packaging.md`, which it revises.
It settles what ADR-0027 §7 and ADR-0028 left to packaging: the bundle identifier, and removing a Scout's alarm when JobPilot is uninstalled.

## Context

JobPilot runs today from a checkout: `make build` for the daemon and CLI, `bun tauri dev` for the desktop.
Nothing has been installed on a machine that did not build it.
Installing it turns up seven things.

### The identifier is a placeholder, and it is in two places

`com.user.jobpilot` is Tauri's template identifier, in `desktop/src-tauri/tauri.conf.json` and as the launchd label prefix in `internal/adapters/secondary/alarms/launchd.go`.
It names the macOS agents, the webview's storage, and the app macOS asks permission to notify for.
The data directory (`os.UserConfigDir()/jobpilot`) and the keychain service (`jobpilot`) do not depend on it.

### The app already looks where a bundle puts the daemon

The desktop looks for `jobpilot` next to its own executable before `PATH` (`find_daemon_binary`).
Tauri's `externalBin` places a bundled binary exactly there: `Contents/MacOS` on macOS, the install directory on Windows, `/usr/bin` for a Linux package.
A Scout's alarm points at the daemon by `os.Executable()`, so it points inside the installed app.

### Nothing knows which version it is

No binary reports a version, and `/v1/health` answers only that the daemon is up.
The daemon outlives the desktop (ADR-0027, "The daemon outlives the desktop"), so after an update the new app finds the old daemon still running and uses it.
On Windows the installer cannot overwrite a `jobpilot.exe` that is running.

### Most uninstalls run nothing

A macOS app is uninstalled by moving it to the Trash.
A `.deb` or `.rpm` runs its scripts as root, and a Scout's alarm belongs to the user, in their own `~/.config/systemd/user`.
Only a Windows NSIS installer runs an uninstaller as the user who installed it.
An alarm left behind fires at every scheduled time and fails, forever (ADR-0027, Consequences).

### An AppImage cannot hold a daemon

An AppImage runs from a mount that exists while the AppImage's own process does, at a path that changes on every launch.
A daemon started from it outlives that process and loses its executable, and an alarm written from it points at a path that will not exist.

### Every Go dependency is pure Go

SQLite is `modernc.org/sqlite`, PDFium runs as WebAssembly, the keystore shells out to each platform's tool.
`CGO_ENABLED=0 go build` cross-compiles both binaries for every target from one machine.

### Playwright fetches its driver when first used

`playwright.Install` downloads the Playwright driver the first time a browser is opened, and never the browser itself, which is the user's installed Chrome.

## Decision

### 1. The identifier is `io.github.unicdeve.jobpilot`

The reverse of a namespace the project already controls, its GitHub Pages domain, so it never has to change.
It replaces `com.user.jobpilot` in the Tauri configuration and the launchd label prefix.

The alarms adapter lists agents under both prefixes for as long as the old one is recognised, so the first reconcile after the change removes the old agents and writes new ones.
The old prefix is recognised until the first release, then removed: nothing released ever carried it.

The webview's storage starts empty under the new identifier, so the theme preference is chosen again once, and macOS asks again for permission to notify.
Data and API keys are unaffected.

### 2. One version, set by the release tag

A release is a tag `vX.Y.Z`.
`tauri.conf.json`, `Cargo.toml` and `package.json` carry the same version, and the release workflow refuses a tag that disagrees with any of them.
The Go binaries are given it at build time, with `-ldflags -X`, in one package both import.
A binary built any other way, including by `make build` and `make dev`, says `dev`.

`jobpilot --version` and `jobpilot-cli --version` print it, `/v1/health` returns it, and the desktop shows it under Settings → About, beside the daemon's.

### 3. The app carries the daemon and the CLI

| | Desktop | Format | Architectures |
| --- | --- | --- | --- |
| macOS | `JobPilot.app` in a `.dmg` | universal | arm64 and x86_64 in one |
| Windows | NSIS installer, per user | `.exe` | x86_64 |
| Linux | `.deb` and `.rpm` | packages | x86_64 and arm64 |

`jobpilot` and `jobpilot-cli` are Tauri sidecars in every one of them, next to the app's executable.
On Linux that puts both on `PATH`.
On macOS and Windows the CLI is inside the app, and the README says how to put it on `PATH`.

Windows installs per user, with no administrator prompt: the data directory, the Task Scheduler tasks and the daemon are all the user's.
There is no MSI, which installs per machine and has no hooks for decision 5.
There is no AppImage (Context, "An AppImage cannot hold a daemon").

The CLI and daemon are also released alone, as a `.tar.gz` or `.zip` per platform and architecture, for a machine without the desktop.

Both binaries are built by `go build` from the Makefile (`make dist`), with `CGO_ENABLED=0`.
`packaging.md` recommended GoReleaser for them; one Makefile target is the same build locally and in CI, where GoReleaser would be a second definition of it.

### 4. A newer client replaces an older daemon

Every client compares its version with the daemon's when it finds one: the desktop, the CLI, and `jobpilot scout wake`.

- **The client is newer.** It asks the daemon to stop, waits for it to go, and starts its own. The daemon stops as it does on `SIGTERM`, which waits for a Scout run and for notification delivery to finish (ADR-0027 §4, ADR-0028).
- **The client is older.** It uses the daemon, and the CLI says that it is older than the daemon.
- **Either says `dev`.** They match, so `make dev` and `tauri dev` work against each other as they do today.

Versions are compared as semantic versions, so a downgraded app never stops a newer daemon.

Stopping is `POST /v1/shutdown`, and `jobpilot stop`, which finds the daemon by its discovery file and asks.
It exposes nothing the API does not already: anything that can reach the daemon can already delete all data (ADR-0012).

A daemon whose database has migrations it does not know refuses to start, and says that the data was written by a newer JobPilot.
A release never migrates downward.

### 5. Nothing JobPilot schedules outlives it

**Windows.** The NSIS installer's hooks run, as the user:

- before installing: the installed `jobpilot.exe stop`, if there is one, so an upgrade can replace it;
- before uninstalling: `jobpilot.exe stop`, then `jobpilot.exe alarms remove`, which removes every JobPilot task.

**macOS and Linux.** No uninstaller runs as the user, so each alarm checks for its own executable.
The entry runs a short `/bin/sh` script: if the executable is gone, it removes its own entry and exits; otherwise it runs `jobpilot scout wake <id>` in its place, as today.
Moving the app does the same, and the daemon writes the alarms again at the new path the next time it starts, as ADR-0027 §6 already does for a moved executable.

The macOS agent names the app in `AssociatedBundleIdentifiers`, so System Settings → Login Items shows JobPilot rather than `sh`.
That is measured on a built app before it is relied on.

An uninstaller never deletes data.
Settings → Data → "Delete all data" is how data is removed (ADR-0021), and the README says so beside the uninstall steps.

### 6. Releases are unsigned, and ready to be signed

**macOS.** The app is ad-hoc signed, which Apple Silicon requires to run anything.
A downloaded copy is quarantined, and opens once the user chooses Open Anyway in System Settings → Privacy & Security.
**Windows.** SmartScreen warns about an unrecognised publisher, and the installer runs after More info → Run anyway.

The README and every release's notes say both, with what each dialog looks like.

The release workflow signs whenever its signing secrets are present: an Apple Developer ID certificate and notarization credentials, and a Windows signing command.
Signing later is configuration, not a change to the workflow.

Whether a quarantined, ad-hoc signed app can start its own sidecars, and launchd start them, without asking again is measured on a downloaded build before the first release.

### 7. Updating is manual

A new version is a GitHub Release, with notes saying what changed.
The user downloads and installs it over the old one; decision 4 replaces the running daemon.
JobPilot itself makes no request to find out whether there is a newer version.

In-app update checking, signed update artifacts and installing on the user's say-so stay post-MVP (`packaging.md`), and are a later ADR, since a check would be JobPilot's first request to anything other than a job source, a model, or a site the user sends it to.

### 8. GitHub Actions builds every change and every release

**On every push and pull request**, `ci.yml`:

- `make check` on Linux;
- the Go unit and integration tests on macOS and Windows;
- the desktop: `bun run build`, and `cargo clippy` with warnings as errors.

**On a `v*` tag**, `release.yml`:

- checks the tag against the three version fields (decision 2);
- runs the checks above;
- builds the sidecars for each target, then the desktop with Tauri's own action;
- builds the standalone CLI archives;
- writes `SHA256SUMS` over every artifact;
- creates a **draft** GitHub Release with all of them attached.

The user publishes the draft.
Before tagging, the release checklist in `packaging.md` runs `make starter-list` by hand, as it already says, since it reads every board over the network.

### 9. No login item

The roadmap's "OS startup integration" is the Scout alarms ADR-0027 built: a scheduled Scout starts JobPilot after a restart without the user opening anything.
ADR-0027 chose those over starting the daemon at login (option C), and that stands.
The daemon still starts on demand (ADR-0009).

### 10. Playwright's driver is still fetched on first use

The installers do not carry it.
Preparing an application in a browser needs the network anyway, to reach the job's site, so fetching the driver then costs nothing a user could have done offline.
The first "Prepare in browser" says it is downloading what it needs.

## Consequences

- `tauri.conf.json` gains the identifier, `externalBin`, the NSIS hooks and per-user install mode, and the `deb` and `rpm` settings; `bundle.targets` names those formats rather than `all`.
- The daemon gains `stop` and `alarms remove` beside `scout wake`, a `--version` flag, and `POST /v1/shutdown`. `/v1/health` returns a version.
- The daemon-finding code in Go (`internal/adapters/primary/httpapi/ensure.go`) and in Rust (`desktop/src-tauri/src/daemon.rs`) both learn decision 4, and stay in step as they do today.
- Every alarm's entry changes shape, from the executable to a script that runs it. The fingerprint changes with it, so reconcile rewrites every existing entry once.
- The repository gains `.github/workflows/`, and `make dist`.
- CI runs the existing tests on Windows and macOS for the first time. It does not run the alarm or keystore adapters against a real scheduler or keychain, which stay listed in `current_state.md` as unverified until measured on each system.
- A notification from a built app, attributed to JobPilot, becomes measurable (ADR-0028, Consequences).
- `packaging.md` is rewritten around this: what each release contains, the release checklist, and GoReleaser removed.
- The README gains installing, opening an unsigned build, updating, and uninstalling.
- Until signing, every macOS user goes through Open Anyway once per download, and every Windows user through SmartScreen.

## Alternatives considered

**`com.user.jobpilot`, kept.** No change, but it is a template's, in a namespace nobody owns.

**A domain bought for the project.** Equally stable, at the cost of renewing it forever, and nothing else needs one yet.

**Signing now.** Removes the Gatekeeper and SmartScreen steps for every user, for $99 a year (Apple) and a Windows signing service, before anyone has asked for a build.

**An opt-in update check now.** Tauri's updater with its own key. Deferred with decision 7's reason.

**GoReleaser for the Go binaries.** What `packaging.md` recommended. A second build definition beside the Makefile, for binaries one `go build` per target already makes.

**An AppImage.** Runs on any distribution without a package, but cannot hold a daemon (Context).

**An MSI on Windows.** Installs per machine, needs an administrator, and runs no hooks as the user.

**The daemon outside the app, installed separately.** Two things to install and keep in step, for the same binary the app can carry.

**Installers delete JobPilot's data.** A reinstall would lose everything, and deletion has its own previewed path (ADR-0021).

**Alarms that never check their executable.** Simpler entries, and an uninstalled JobPilot's alarms fail every hour until the user finds them in System Settings or `systemctl`.
