# Agent Architecture Overview

JobPilot coordinates explicit core services rather than a general model-directed tool loop.
Scouts discover jobs, score them, apply deterministic eligibility rules and create applications for review.
Browser preparation follows a bounded navigation and form-filling workflow and leaves submission to the user.
Both workflows persist structured activity events.

Models score matches, draft content and, when deterministic navigation fails, propose an apply control from the page's existing controls.
Core validates model output and enforces permissions and policy.
The model has no unrestricted system access.

The general tool runtime described in [Agent Loop](01_agent_loop.md) and [Agent Tools](02_tools.md) is a design proposal, not the current implementation.
It cannot acquire a submission capability: ADR-0014 forbids JobPilot from clicking an external application's submit control.
