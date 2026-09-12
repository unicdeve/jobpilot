# Testing Strategy

## Priority

Test core logic, not every UI pixel.

## High-value tests

- candidate profile validation
- job normalization
- job deduplication
- match scoring/structured output validation
- application state transitions
- policy evaluation
- prompt resolution
- answer resolution
- data deletion
- configuration validation
- migration correctness

## Integration tests

Add selectively for:
- SQLite repositories
- LLM provider adapter behavior using mocks/fakes
- connector parsing
- the daemon's HTTP API (`internal/adapters/primary/httpapi`): real client
  against a real `httptest` server, with fake ports/adapters underneath —
  see `server_test.go` in that package

## Browser tests

`internal/adapters/secondary/browser/playwright_worker_test.go` (tagged
`integration`) runs a real headless Chrome against a local `httptest`-served
fixture form — no live internet site or visible window needed, so it's
deterministic and safe for CI/local pre-push. `internal/core/services/browserworker_test.go`
covers the orchestration logic (extraction → question → resolve → fill,
blocker/failure handling, state transitions) with a fake `ports.BrowserWorker`
and needs no browser at all. The first run of the browser-adapter test
downloads the Playwright driver (~50MB, cached afterward) — expect that run
to take longer.

## Running tests

Unit tests (`internal/core/services`, plus pure-logic files like
`sqlite/utils.go` and `llm/presets.go`) run with no build tag:

```bash
go test ./...
```

Integration tests (SQLite repositories against a real temp-file database;
LLM provider adapters against a fake HTTP server via `httptest`) are tagged
`//go:build integration` and run with:

```bash
go test -tags=integration ./...
```

CI and local pre-push checks should run the tagged variant (optionally with
`-race`) to cover the full suite; `golangci-lint` also needs
`--build-tags=integration` to lint the tagged files. `make lint` and `make
lint-integration` cover both, and `make check` runs the whole gate:
formatting, `go vet` (tagged and untagged), both lint passes, and both test
suites.

## Tooling

`golangci-lint` is pinned by `GOLANGCI_LINT_VERSION` in the `Makefile` and
installed into `./bin` (gitignored) on demand — `make tools`, or implicitly
by any lint target. Nothing needs to be installed on the machine beyond Go
itself, and every contributor lints against the same rule set, since a
different linter version reports a different set of problems.

It is reinstalled when the Go toolchain changes, not only when
`GOLANGCI_LINT_VERSION` does. golangci-lint type-checks with the `go/types`
it was compiled against, so a binary built by an older Go cannot parse a
newer standard library and reports errors that are not in your code — going
from Go 1.26 to 1.27 produced two invented failures inside
`crypto/internal/randutil`. `make tools` compares the Go version the binary
was built with and rebuilds it, so that failure mode never reaches anyone.

It is deliberately not a `go.mod` tool dependency: `go install pkg@version`
builds it in isolation from this module, whereas a `tool` directive would
pull golangci-lint's dependency tree into `go.mod` as indirect requires and
let its constraints raise shared dependencies for the application itself.
`go.mod` stays the application's dependency contract.

The enabled linters live in `.golangci.yml`.
