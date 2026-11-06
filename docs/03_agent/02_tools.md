# Agent Tools

These names describe a proposed tool interface, not an implemented registry.
Current capabilities are exposed through core services and the daemon API.

Conceptual local and discovery tools:

- search_jobs
- get_job
- analyze_job
- get_candidate_profile
- get_resume
- save_application_content
- request_user_approval
- notify_user

Conceptual browser tools (the browser only reads a job page for import, and never touches an application form; ADR-0034):

- browser_open
- browser_navigate

Submission is never a tool capability (ADR-0014).
Only the user submits in the browser JobPilot leaves open.

Each proposed tool must define:
- name
- description
- input schema
- output schema
- permission/risk
- side effects
- failure modes
