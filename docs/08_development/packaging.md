# Packaging and Updating

## Goals

Provide installable binaries for:
- macOS
- Linux
- Windows

## Release

Use GitHub Releases as the initial distribution mechanism.

Before each release, run `make starter-list` and commit the regenerated `internal/core/domain/starter_companies_gen.go`, so no company JobPilot suggests watching was measured before the release that ships it (`docs/02_features/17_company_watchlists.md`).

Recommended build/release tooling:
- GoReleaser for CLI/daemon artifacts
- Tauri bundling for desktop installers
- GitHub Actions for reproducible builds

## Updating

MVP:
- versioned releases
- clear manual update instructions

Post-MVP:
- optional in-app update checking
- signed update artifacts
- user-controlled update installation

Do not build a custom update server for MVP.
