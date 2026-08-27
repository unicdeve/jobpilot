# Tool Permissions

## Categories

READ_LOCAL
READ_EXTERNAL
WRITE_LOCAL
EXTERNAL_SIDE_EFFECT

## Rules

LLM output cannot grant itself permissions.

The runtime determines whether a tool is available.

External side effects require policy evaluation and, for Review mode, explicit user approval.
