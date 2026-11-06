# JobPilot

Open-source, local-first AI job search and application assistant.

JobPilot discovers jobs, explains how they match your profile and resume, and keeps track of the applications you send.
It keeps your data on your computer and offers a Go CLI and a Tauri desktop app for macOS, Linux and Windows.

**You write and send your own applications.**
JobPilot never writes a cover letter or an answer for you, and never fills in or submits an employer's form ([ADR-0034](docs/09_decisions/0034-the-candidate-writes-and-fills-their-own-application.md)).
It opens the application page in your own browser and keeps your details, letter and answers ready to copy.
A Scout runs a saved search, scores jobs against deterministic eligibility rules, and creates applications for you to work on, without contacting employers.

## Current status

The main product workflows and packaging tooling are implemented; the first release and installer validation are still outstanding.
Build from source using the instructions below until release artifacts are published.
See [current state](docs/00_product/current_state.md) for implementation gaps and [roadmap](docs/00_product/roadmap.md) for planned work.
The plugin ecosystem is post-MVP.

## What you can do

- Keep your profile, resumes, job history and applications locally.
- Discover and deduplicate jobs from public boards and watched company hiring pages.
- Match jobs against your profile and resume, with structured explanations.
- Write your cover letter and answers beside the job, with your profile details one click from the clipboard and checks that point out stock phrases and unsupported claims.
- Record what happened after you applied: interviewing, offered, rejected or no response.
- Run Scouts manually or on an operating-system schedule, with local notifications.
- Configure model providers, prompt guidance and saved searches through the desktop or CLI.

Ollama is the default local model provider; other local servers and hosted models are supported.
Using a hosted model sends candidate context and job postings to that provider, which JobPilot identifies in its settings.
Fresh job discovery needs internet access; stored history remains available offline.
The model scores and explains matches; it writes nothing that goes to an employer, and deterministic rules take precedence over it.

## Installing

Once a release is published, download the file for your system from [GitHub Releases](https://github.com/unicdeve/jobpilot/releases).
The packaging workflow produces the following artifacts.

| System | File |
| --- | --- |
| macOS 12 or later, Apple silicon or Intel | `JobPilot_<version>_universal.dmg` |
| Windows 10 or later | `JobPilot_<version>_x64-setup.exe` |
| Debian, Ubuntu | `JobPilot_<version>_amd64.deb`, or `_arm64.deb` |
| Fedora, openSUSE | `JobPilot-<version>-1.x86_64.rpm`, or `.aarch64.rpm` |

- **macOS:** Open the `.dmg` and drag JobPilot into Applications.
- **Windows:** Run the installer.
  It installs JobPilot for you alone, so it never asks for an administrator.
- **Linux:** Install the package with `sudo apt install ./JobPilot_<version>_amd64.deb` or `sudo dnf install ./JobPilot-<version>-1.x86_64.rpm`.

Install Google Chrome to import a job from a source JobPilot does not support.
JobPilot downloads its Playwright driver on first browser use, so that step needs internet access.

The first time JobPilot opens, it asks which AI model to use, a local one such as [Ollama](https://ollama.com) or a hosted one with an API key, then for your profile, your resume and a job search.

### Opening it the first time

The initial packaging configuration produces unsigned releases; opening a downloaded build requires the following system confirmation.

- **macOS:** macOS says it cannot verify that JobPilot is free of malware.
  Choose Done, open System Settings → Privacy & Security, and choose Open Anyway beside the message about JobPilot.
  You do this once for each download.
- **Windows:** SmartScreen says it protected your PC.
  Choose More info, then Run anyway.

Every release lists each file's checksum in `SHA256SUMS`.
Check a download with `shasum -a 256 -c SHA256SUMS --ignore-missing` on macOS, `sha256sum -c SHA256SUMS --ignore-missing` on Linux, or `Get-FileHash <file>` in PowerShell.

### The command line

The desktop app carries the command line interface, `jobpilot-cli` ([docs/CLI.md](docs/CLI.md)).

- **Linux:** The package puts it on your `PATH`.
- **macOS:** Link it onto your `PATH`:

  ```sh
  sudo mkdir -p /usr/local/bin
  sudo ln -s /Applications/JobPilot.app/Contents/MacOS/jobpilot-cli /usr/local/bin/jobpilot-cli
  ```
- **Windows:** Add the folder JobPilot is installed in to your `PATH`, in PowerShell:

  ```powershell
  [Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path", "User") + ";$env:LOCALAPPDATA\JobPilot", "User")
  ```

On a machine without the desktop app, download `jobpilot-cli_<version>_<os>_<arch>` instead.
The `.tar.gz` or `.zip` archive holds `jobpilot-cli` and the background service, `jobpilot`, which the CLI starts; keep the two in the same folder on your `PATH`.
Run `jobpilot-cli onboard` to configure the model, profile, optional resume and first saved search.
On macOS, a downloaded copy has to be cleared to run: `xattr -d com.apple.quarantine jobpilot jobpilot-cli`.

## Updating

Install the new release over the old one, as you installed it.
Your data stays where it is, and the new version replaces the background service the old one left running.

JobPilot never checks for updates itself.
To hear about a release, watch the repository on GitHub for releases (Watch → Custom → Releases).

Going back to an older version is not supported: an older JobPilot refuses data that a newer one has upgraded.

## Uninstalling

Uninstalling leaves your data in place.
JobPilot's background service keeps running after you close the app, so on macOS and Linux you stop it before uninstalling; the Windows uninstaller stops it itself.
To remove your data too, first choose Settings → Data → Delete all data in the app, or run `jobpilot-cli data delete-all`.

- **macOS:** Quit JobPilot, stop its background service with `/Applications/JobPilot.app/Contents/MacOS/jobpilot stop`, and move JobPilot from Applications to the Trash.
  A scheduled scout removes its own alarm the next time it would have run.
- **Windows:** Uninstall JobPilot from Settings → Apps → Installed apps.
  The uninstaller stops JobPilot's background service and removes its scheduled scouts.
  Its "Delete the application data" box clears only the app window's own settings, such as the theme, not your JobPilot data.
- **Linux:** Stop the background service with `jobpilot stop`, then run `sudo apt remove job-pilot` or `sudo dnf remove job-pilot`.
  A scheduled scout removes its own timer the next time it would have run.

Your data is in `~/Library/Application Support/jobpilot` on macOS, `%AppData%\jobpilot` on Windows, and `~/.config/jobpilot` on Linux.

## Development

The core and CLI use Go, persistence uses SQLite, and the desktop uses Tauri 2, React and TypeScript.
Reading a job page for import uses Go Playwright with installed Chrome.

For the CLI, install Go at the version required by [go.mod](go.mod) or later, then run:

```sh
make build
./bin/jobpilot-cli onboard
./bin/jobpilot-cli search run
```

For the desktop, also install Bun, rustup and Tauri's platform build prerequisites; follow the [desktop setup](desktop/README.md).
After installing desktop dependencies, use two terminals for hot reload:

```sh
make dev          # start the daemon first
make run-desktop  # start the desktop in another terminal
```

| Command | Purpose |
| --- | --- |
| `make check` | Go formatting, vet, lint, unit/integration tests and desktop Biome checks |
| `make fmt` / `make lint-fix` | Apply formatting / automatic lint fixes |
| `cd desktop && bun run build` | Type-check and build the frontend |
| `make bundle` | Build installers for this machine with bundled daemon and CLI |

`make check` does not run the frontend build or Rust checks.
See [testing](docs/08_development/testing.md) for the full validation commands, [hot reload](docs/08_development/hot_reload.md) for daemon restart behavior, and [packaging](docs/08_development/packaging.md) for releases.

## Documentation

- [CLI commands](docs/CLI.md)
- [Specification map](docs/MASTER_SPEC.md)
- [Architecture](docs/01_architecture/00_overview.md) and [accepted decisions](docs/09_decisions/)
- [Contributor instructions](AGENTS.md) and [development setup](.agents/SETUP.md)

## License

JobPilot is licensed under the [Apache License 2.0](LICENSE).
Copyright 2026 Taiwo Ogunola; see [NOTICE](NOTICE).
The third-party software it includes, and their licenses, are listed in [THIRD-PARTY-NOTICES.txt](THIRD-PARTY-NOTICES.txt).

## Third-party data

The countries, cities and time zones JobPilot uses to interpret job locations and working hours are generated from [GeoNames](https://www.geonames.org/), licensed under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).
They are compiled into the daemon (`internal/core/domain/geo`); interpreting a job never contacts GeoNames.
