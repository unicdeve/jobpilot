# CLI Specification

The exact command framework can be chosen during implementation. Cobra is the preferred initial Go CLI framework.

## Commands

```text
jobpilot init
jobpilot status
jobpilot doctor

jobpilot profile show
jobpilot profile edit
jobpilot resume add
jobpilot resume list
jobpilot resume remove

jobpilot search
jobpilot search list
jobpilot search run

jobpilot job list
jobpilot job show <id>
jobpilot job analyze <id>

jobpilot application list
jobpilot application show <id>
jobpilot application generate <job-id>
jobpilot application history

jobpilot llm status
jobpilot llm configure

jobpilot prompt list
jobpilot prompt show <id>
jobpilot prompt set <id>

jobpilot config show
jobpilot config set

jobpilot history
jobpilot data delete
jobpilot logs
```

Commands may evolve. Avoid implementing commands that do not have a product purpose.
