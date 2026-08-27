# AI Development Setup

## First session

Do not modify code during initial project discovery.

Read:

1. `AGENTS.md`
2. `docs/MASTER_SPEC.md`
3. `docs/00_product/00_overview.md`
4. `docs/00_product/current_state.md`
5. `docs/01_architecture/00_overview.md`
6. `docs/09_decisions/`

Then inspect the repository.

Report:

- product understanding
- current architecture
- current implementation state
- current phase
- relevant decisions
- missing or conflicting requirements
- recommended next task

## Feature implementation

For a requested feature:

1. Read the feature specification.
2. Read referenced architecture documents.
3. Read relevant rules and skill.
4. Inspect current implementation.
5. Produce an implementation plan.
6. Identify tests.
7. Identify documentation changes.
8. Wait for approval when the workflow calls for approval.
9. Implement only the approved scope.
10. Run tests and checks.
11. Verify acceptance criteria.
12. Update documentation/state when required.

## Source of truth

Priority order:

1. Explicit current user instruction
2. Accepted ADRs
3. Product requirements
4. Architecture documents
5. Feature specifications
6. AI rules and skills
7. Existing implementation
8. Agent assumptions

When sources conflict, do not silently choose. Surface the conflict.
