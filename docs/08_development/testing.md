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
