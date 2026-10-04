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

`desktop/src-tauri/tauri.conf.json`, `desktop/src-tauri/Cargo.toml` and `desktop/package.json` carry the release's version, without the `v`.
The release workflow refuses a tag that disagrees with any of them.

A newer app or CLI replaces an older daemon it finds running; an older one uses a newer daemon, and the CLI says it is older (ADR-0029 §4).
A daemon refuses to open a database written by a newer JobPilot.
Releases never migrate a database downward, so going back a version means restoring data from before the upgrade.

## Releasing

1. Run `make starter-list` and commit the regenerated `internal/core/domain/starter_companies_gen.go`, so no company JobPilot suggests watching was measured before the release that ships it (`docs/02_features/17_company_watchlists.md`).
   It reads every board over the network, which is why CI does not run it.
2. Set the version in the three files above, and commit.
3. Tag the commit `vX.Y.Z` and push the tag.
4. `.github/workflows/release.yml` checks the tag against the version, runs the checks CI runs, builds every artifact, and creates a **draft** GitHub Release with them attached.
5. Download the draft's builds and install them on each platform at hand.
6. Write the release notes: what changed, anything the user must do, and the unsigned-build steps below.
7. Publish the draft.

Publishing is always done by hand.

## Signing

Releases are unsigned until signing is paid for (ADR-0029 §6).

- **macOS.** The app is ad-hoc signed. A downloaded copy opens after System Settings → Privacy & Security → Open Anyway, once per download.
- **Windows.** SmartScreen warns about an unrecognised publisher. The installer runs after More info → Run anyway.

The README and every release's notes say both.

The release workflow signs whenever its signing secrets are present: an Apple Developer ID certificate with notarization credentials, and a Windows signing command.
Turning signing on is adding those secrets.

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
