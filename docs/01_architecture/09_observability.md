# Observability

## Goals

Make agent behavior debuggable without exposing sensitive data.

## Required

- Structured application logs
- Agent run IDs
- Tool invocation events
- Success/failure state
- Timing where useful
- Error context
- User-visible activity history

## Sensitive data

Avoid logging:
- API keys
- tokens
- passwords
- full resumes
- full application answers unless explicitly required for local debugging

## Agent trace

Structured browser and scout traces are implemented as `AgentRun` and `AgentEvent` records.
Users can read them in desktop Activity or `jobpilot-cli activity`.
Events include:

- action
- tool
- input summary
- output summary
- policy result
- timestamp
- status

Do not expose or persist hidden chain-of-thought.
