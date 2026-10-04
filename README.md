# JobPilot

Open-source, local-first AI job search and application assistant.

JobPilot helps users discover relevant jobs, evaluate them against their
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

JobPilot also fills in an application form in a browser window it leaves open
for you, and you submit it yourself; it never submits an application
([ADR-0014](docs/09_decisions/0014-scouts-and-no-unattended-submission.md)).
Extensive third-party integrations and the plugin ecosystem are post-MVP.

## Installing

Download the file for your system from the [latest release](https://github.com/unicdeve/jobpilot/releases/latest).

| System | File |
| --- | --- |
| macOS 12 or later, Apple silicon or Intel | `JobPilot_<version>_universal.dmg` |
| Windows 10 or later | `JobPilot_<version>_x64-setup.exe` |
| Debian, Ubuntu | `JobPilot_<version>_amd64.deb`, or `_arm64.deb` |
| Fedora, openSUSE | `JobPilot-<version>-1.x86_64.rpm`, or `.aarch64.rpm` |

- **macOS.** Open the `.dmg` and drag JobPilot into Applications.
- **Windows.** Run the installer. It installs JobPilot for you alone, so it never asks for an administrator.
- **Linux.** Install the package with `sudo apt install ./JobPilot_<version>_amd64.deb` or `sudo dnf install ./JobPilot-<version>-1.x86_64.rpm`.

The first time JobPilot opens, it asks which AI model to use, a local one such as [Ollama](https://ollama.com) or a hosted one with an API key, then for your profile, your resume and a job search.

### Opening it the first time

Releases are not signed yet, so your system asks once whether to trust JobPilot.

- **macOS.** macOS says it cannot verify that JobPilot is free of malware. Choose Done, open System Settings → Privacy & Security, and choose Open Anyway beside the message about JobPilot. You do this once for each download.
- **Windows.** SmartScreen says it protected your PC. Choose More info, then Run anyway.

Every release lists each file's checksum in `SHA256SUMS`.
Check a download with `shasum -a 256 -c SHA256SUMS --ignore-missing` on macOS, `sha256sum -c SHA256SUMS --ignore-missing` on Linux, or `Get-FileHash <file>` in PowerShell.

### The command line

The desktop app carries the command line interface, `jobpilot-cli` ([docs/CLI.md](docs/CLI.md)).

- **Linux.** The package puts it on your `PATH`.
- **macOS.** Link it onto your `PATH`:
  ```sh
  sudo mkdir -p /usr/local/bin
  sudo ln -s /Applications/JobPilot.app/Contents/MacOS/jobpilot-cli /usr/local/bin/jobpilot-cli
  ```
- **Windows.** Add the folder JobPilot is installed in to your `PATH`, in PowerShell:
  ```powershell
  [Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path", "User") + ";$env:LOCALAPPDATA\JobPilot", "User")
  ```

On a machine without the desktop app, download `jobpilot-cli_<version>_<os>_<arch>` instead.
It holds `jobpilot-cli` and the background service, `jobpilot`, which the CLI starts; keep the two in the same folder on your `PATH`.
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
To remove it too, first choose Settings → Data → Delete all data in the app, or run `jobpilot-cli data delete-all`.

- **macOS.** Quit JobPilot, stop its background service with `/Applications/JobPilot.app/Contents/MacOS/jobpilot stop`, and move JobPilot from Applications to the Trash. A scheduled scout removes its own alarm the next time it would have run.
- **Windows.** Uninstall JobPilot from Settings → Apps → Installed apps. The uninstaller stops JobPilot's background service and removes its scheduled scouts. Its "Delete the application data" box clears only the app window's own settings, such as the theme, not your JobPilot data.
- **Linux.** Stop the background service with `jobpilot stop`, then run `sudo apt remove job-pilot` or `sudo dnf remove job-pilot`. A scheduled scout removes its own timer the next time it would have run.

Your data is in `~/Library/Application Support/jobpilot` on macOS, `%AppData%\jobpilot` on Windows, and `~/.config/jobpilot` on Linux.

## Documentation

Start with `AGENTS.md`, then `.agents/SETUP.md` and `docs/MASTER_SPEC.md`. For a
list of available CLI commands, see [docs/CLI.md](docs/CLI.md).

## Development

The core runtime is Go. The desktop UI uses Tauri + React + TypeScript. SQLite
is the local database. Browser automation uses Go Playwright.

See `docs/01_architecture/` for the architecture and `docs/09_decisions/` for
accepted decisions.

`make dev` runs the daemon with hot reload, and `make run-desktop` the desktop
alongside it; see [docs/08_development/hot_reload.md](docs/08_development/hot_reload.md).
`make check` runs every check the Definition of Done asks for.
`make bundle` builds the desktop's installers for this machine, and
[docs/08_development/packaging.md](docs/08_development/packaging.md) describes
releases.

## License

JobPilot is licensed under the [Apache License 2.0](LICENSE).
Copyright 2026 Taiwo Ogunola; see [NOTICE](NOTICE).
The third-party software it includes, and their licenses, are listed in
[THIRD-PARTY-NOTICES.txt](THIRD-PARTY-NOTICES.txt).

## Third-party data

The countries, cities and time zones JobPilot reads job locations and hours
against are generated from [GeoNames](https://www.geonames.org/), licensed under
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). They are compiled
into the daemon (`internal/core/domain/geo`), and placing a job never contacts
GeoNames.
