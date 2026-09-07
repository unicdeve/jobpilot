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

## Browser tests

Post-MVP and focused on critical automation behavior.

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
