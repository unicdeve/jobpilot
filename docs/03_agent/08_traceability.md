# Agent Traceability

Persist structured events, not hidden chain-of-thought.

Browser preparation and scout runs already persist `AgentRun` and `AgentEvent` records.
The desktop Activity screen and `jobpilot-cli activity` expose these records.

An event includes:
- run ID
- action
- tool
- input summary
- output summary
- policy result
- status
- duration
- timestamp

Run failures are recorded on the run; event summaries explain individual steps.
Filled-field events omit the candidate's answer.

The trace should allow users/developers to understand what happened without exposing internal model reasoning.
