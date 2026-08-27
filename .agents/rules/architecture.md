# Architecture Rules

- Keep domain logic independent of UI.
- Keep external integrations behind interfaces.
- Keep LLM provider details behind the LLM abstraction.
- Keep browser details behind browser interfaces.
- Prefer local process boundaries over network services for local-only components.
- Avoid premature distributed architecture.
