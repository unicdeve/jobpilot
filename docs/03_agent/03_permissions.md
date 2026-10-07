# Tool Permissions

These are conceptual categories for a future general tool runtime.
Current services enforce their own explicit boundaries; a Scout cannot enter the browser, and Assist mode cannot prepare an external form.

## Categories

READ_LOCAL
READ_EXTERNAL
WRITE_LOCAL
EXTERNAL_SIDE_EFFECT

## Rules

LLM output cannot grant itself permissions.

The runtime determines whether a tool is available.

Consequential external actions require explicit user approval.
No permission, approval or mode authorizes JobPilot to submit an application (ADR-0014).
