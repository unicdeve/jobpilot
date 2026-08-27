# Packaging and Updating

## Goals

Provide installable binaries for:
- macOS
- Linux
- Windows

## Release

Use GitHub Releases as the initial distribution mechanism.

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
