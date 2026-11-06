# Threat Model

## Assets

- candidate profile
- resumes
- contact information
- application history
- the candidate's application content
- LLM API keys
- browser sessions
- plugin permissions if external plugins are introduced

## Threats

### Malicious webpage content

A job page may contain text designed to manipulate the model.

Mitigation:
- treat webpage text as untrusted data
- separate system instructions from external content
- validate tool actions outside the model

The same text is also *displayed*, which is a second surface: a job
description is stored as markdown and rendered on the desktop's job detail
screen (`desktop/src/components/markdown.tsx`).

Mitigation:
- raw HTML in that markdown is never rendered, so script tags and inline
  event handlers cannot execute
- images are not loaded, only their alt text is shown — a remote image is a
  request to a third-party server disclosing which posting the user is
  reading, which `docs/07_security/01_privacy.md` does not permit
- links are restricted to http(s) and opened in the user's browser rather
  than navigated to in the webview, which would otherwise replace the
  application with the job board

### Malicious plugin (future extension loading)

External plugin loading is not implemented.
Any future runtime must provide these mitigations:
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
- no JobPilot code path clicks an external application's submit control (ADR-0014)
- no JobPilot code path fills in an external application form; the user applies in their own browser (ADR-0034)
- Scouts open no browser

Marking an application ready to send, or sent, is a local record, never a command to an employer.

### Local data loss

Mitigation:
- safe migrations
- export/backup strategy post-MVP
- user-controlled deletion

## Security invariant

The LLM is never the final authority for permissions or external side effects.
