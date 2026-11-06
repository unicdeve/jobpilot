# Agent Architecture Overview

JobPilot coordinates explicit core services rather than a general model-directed tool loop.
Scouts discover jobs, score them, apply deterministic eligibility rules and create applications for the candidate, and persist structured activity events.
The candidate writes and sends their own applications; no workflow writes application content or touches an employer's form (ADR-0034).

Models score matches and read resumes and job pages; they draft nothing for an employer.
Core validates model output and enforces permissions and policy.
The model has no unrestricted system access.

The general tool runtime described in [Agent Loop](01_agent_loop.md) and [Agent Tools](02_tools.md) is a design proposal, not the current implementation.
It cannot acquire a submission capability: ADR-0014 forbids JobPilot from clicking an external application's submit control.
