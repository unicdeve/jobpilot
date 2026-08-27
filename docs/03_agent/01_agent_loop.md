# Agent Loop

## Steps

1. Load task.
2. Load relevant candidate/job context.
3. Determine available tools.
4. Ask LLM for a structured next action.
5. Validate action against tool schema.
6. Validate permissions/policy.
7. Execute tool.
8. Validate tool result.
9. Persist structured event.
10. Continue until task completes, requires user input, or fails safely.

## Limits

The runtime must enforce:
- maximum steps
- maximum time
- cancellation
- provider failure handling
- tool permissions
