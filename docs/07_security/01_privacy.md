# Privacy Model

## Default

User data stays on the local machine.

## Data that may leave the machine

Only data required by an explicitly configured external service, such as:
- cloud LLM provider
- email provider
- remote job source

## UX

When a cloud LLM is configured, clearly communicate that relevant prompt context can be sent to that provider.

## Telemetry

No analytics/usage telemetry is required for MVP. If telemetry is ever introduced, it should be opt-in, documented, minimal, and disabled by default.
