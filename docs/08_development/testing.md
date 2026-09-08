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
`--build-tags=integration` to lint the tagged files.
