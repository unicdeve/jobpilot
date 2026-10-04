# Packaging and Updating

How JobPilot is built into something a user installs, released, and updated.
The decisions are ADR-0029's; this document is how to carry them out.
`docs/00_product/current_state.md` says which parts are built.

## What a release contains

A release is a git tag `vX.Y.Z` and a GitHub Release built from it.

| | Desktop | Format | Architectures |
| --- | --- | --- | --- |
| macOS | `JobPilot.app` in a `.dmg` | universal | arm64 and x86_64 in one |
| Windows | NSIS installer, per user | `.exe` | x86_64 |
| Linux | `.deb` and `.rpm` | packages | x86_64 and arm64 |

Every desktop build carries the daemon, `jobpilot`, and the CLI, `jobpilot-cli`, as Tauri sidecars next to the app's executable.
On Linux the package puts both on `PATH`.

The CLI and daemon are also released alone, as a `.tar.gz` (macOS, Linux) or `.zip` (Windows) per platform and architecture.

`SHA256SUMS` lists a checksum for every file in the release.

There is no AppImage and no MSI (ADR-0029 §3).

## Building

The Go binaries are pure Go and built with `CGO_ENABLED=0`, so every target cross-compiles from one machine.
`make dist` builds them, with the version given at build time; GoReleaser is not used (ADR-0029 §3).
A binary built any other way, including `make build` and `make dev`, reports its version as `dev`.

The desktop is built by `make bundle TARGET=<rust target> VERSION=x.y.z` on a machine of each platform: `universal-apple-darwin` on a Mac, `x86_64-pc-windows-msvc` on Windows, and `x86_64-unknown-linux-gnu` or `aarch64-unknown-linux-gnu` on Linux.
It first runs `make sidecars`, which builds the daemon and the CLI into `desktop/src-tauri/binaries/` under the names Tauri's `externalBin` expects (`jobpilot-<target>`), universal on macOS by `lipo` from both architectures, then `tauri build` for that target.
A universal build needs both Rust targets: `rustup target add aarch64-apple-darwin x86_64-apple-darwin`.

The sidecars are named in `desktop/src-tauri/tauri.bundle.conf.json`, which `make bundle` passes to `tauri build --config`, rather than in `tauri.conf.json` with the rest of the bundle's settings.
Tauri refuses to compile the app at all while a binary `externalBin` names is missing, so every `tauri dev`, `cargo clippy` and `cargo test` would otherwise need the sidecars built first, and a development app would start the staged daemon beside it rather than the one `make dev` runs.

The bundle requires macOS 12, which is what Go requires, Windows 10, and on Linux what Tauri's `.deb` and `.rpm` declare.
Windows installs per user (`installMode: currentUser`), with the hooks in `desktop/src-tauri/windows/hooks.nsh`.

## Versions

`desktop/src-tauri/tauri.conf.json`, `desktop/src-tauri/Cargo.toml` and `desktop/package.json` carry the release's version, without the `v`, and so does `desktop/src-tauri/Cargo.lock`, whose entry for the desktop crate every `--locked` build checks against `Cargo.toml`.
`make set-version VERSION=x.y.z` writes it into all four, and `make version-check VERSION=x.y.z` fails unless all four say it.
The release workflow runs the check, so it refuses a tag that disagrees with any of them.

A newer app or CLI replaces an older daemon it finds running; an older one uses a newer daemon, and the CLI says it is older (ADR-0029 §4).
A daemon refuses to open a database written by a newer JobPilot.
Releases never migrate a database downward, so going back a version means restoring data from before the upgrade.

## Releasing

1. Run `make starter-list` and commit the regenerated `internal/core/domain/starter_companies_gen.go`, so no company JobPilot suggests watching was measured before the release that ships it (`docs/02_features/17_company_watchlists.md`).
   It reads every board over the network, which is why CI does not run it.
2. Run `make set-version VERSION=x.y.z`, and commit.
3. Tag the commit `vX.Y.Z` and push the tag.
   A tag with a pre-release, `vX.Y.Z-rc.1`, makes a pre-release.
4. `.github/workflows/release.yml` checks the tag against the version, runs `ci.yml`'s checks, builds every artifact, and creates a **draft** GitHub Release with them attached and `.github/release-notes.md` as its notes.
5. Download the draft's builds and install them on each platform at hand.
6. Write the release notes' "What changed", and anything the user must do.
   The template already says how to open an unsigned build.
7. Publish the draft.

The workflow builds each platform on a machine of its own: the universal macOS app on `macos-latest`, the Windows installer on `windows-latest`, the `.deb` and `.rpm` on `ubuntu-24.04` and, for arm64, `ubuntu-24.04-arm`, and the standalone archives on Linux.
GitHub's arm64 Linux runners are free for public repositories only, so the arm64 job waits for a runner while the repository is private (ADR-0029 makes it public with the first release).

Publishing is always done by hand.

## Signing

Releases are unsigned until signing is paid for (ADR-0029 §6).

- **macOS.** The app is ad-hoc signed. A downloaded copy opens after System Settings → Privacy & Security → Open Anyway, once per download.
- **Windows.** SmartScreen warns about an unrecognised publisher. The installer runs after More info → Run anyway.

The README and every release's notes say both.

The release workflow signs whenever its signing secrets are present, and each reaches the build only when it is set, since Tauri reads an empty one as an identity to sign with.
Turning signing on is adding those secrets to the repository:

- **macOS:** `APPLE_CERTIFICATE` (the Developer ID Application certificate, a base64 `.p12`), `APPLE_CERTIFICATE_PASSWORD` and `APPLE_SIGNING_IDENTITY`, which Tauri signs with in place of the ad-hoc `-`; and `APPLE_ID`, `APPLE_PASSWORD` (an app-specific password) and `APPLE_TEAM_ID`, which it notarizes with.
- **Windows:** `WINDOWS_SIGN_COMMAND`, a command with `%1` for the file to sign, which the workflow gives Tauri as `bundle.windows.signCommand` through `make bundle BUNDLE_FLAGS=--config …`. Tauri splits it on spaces, so a path with a space in it needs a wrapper script.

## Updating

A user updates by installing the new release over the old one.
JobPilot makes no request to find out whether a newer version exists.

In-app update checking, signed update artifacts, and installing on the user's say-so are post-MVP, and need an ADR of their own (ADR-0029 §7).
Do not build a custom update server.

## Uninstalling

Uninstalling never deletes the user's data; "Delete all data" in Settings does (ADR-0021).

A Scout's alarm does not outlive JobPilot (ADR-0029 §5).
The Windows uninstaller stops the daemon and removes every alarm.
On macOS and Linux, where no uninstaller runs as the user, each alarm removes itself the next time it fires and finds JobPilot gone.
