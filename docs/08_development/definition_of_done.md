# Definition of Done

A feature is complete when:

- [ ] Requirements implemented.
- [ ] Acceptance criteria verified.
- [ ] Core logic tests added.
- [ ] Error cases relevant to the feature handled.
- [ ] Existing tests pass.
- [ ] Formatting/linting/static checks pass (`make check`).
- [ ] A change to what ships, a dependency added, removed or upgraded in Go, Rust or the desktop's frontend, has `make notices` run and its output committed; CI's `make notices-check` fails until it has.
- [ ] No unrelated refactor introduced.
- [ ] Security implications considered.
- [ ] Documentation updated if behavior changed.
- [ ] Current state updated when milestone state changed.

## Testing scope

MVP prioritizes tests for core business logic, schemas, state transitions, policy evaluation, matching normalization, and deterministic data handling.

UI/browser end-to-end testing is not a core MVP requirement.
