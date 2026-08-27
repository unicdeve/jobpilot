# Threat Model

## Assets

- candidate profile
- resumes
- contact information
- application history
- generated application content
- LLM API keys
- browser sessions
- plugin permissions

## Threats

### Malicious webpage content

A job page may contain text designed to manipulate the model.

Mitigation:
- treat webpage text as untrusted data
- separate system instructions from external content
- validate tool actions outside the model

### Malicious plugin

Mitigation:
- explicit permissions
- process isolation
- user approval
- plugin provenance where available

### Secret leakage

Mitigation:
- OS keychain
- redacted logs
- no raw password storage

### Unauthorized application submission

Mitigation:
- application modes
- deterministic policy engine
- explicit Review approval

### Local data loss

Mitigation:
- safe migrations
- export/backup strategy post-MVP
- user-controlled deletion

## Security invariant

The LLM is never the final authority for permissions or external side effects.
