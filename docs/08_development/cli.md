# CLI Specification

The exact command framework can be chosen during implementation. Cobra is the preferred initial Go CLI framework.

## Commands

```text
jobagent init
jobagent status
jobagent doctor

jobagent profile show
jobagent profile edit
jobagent resume add
jobagent resume list
jobagent resume remove

jobagent search
jobagent search list
jobagent search run

jobagent job list
jobagent job show <id>
jobagent job analyze <id>

jobagent application list
jobagent application show <id>
jobagent application generate <job-id>
jobagent application history

jobagent llm status
jobagent llm configure

jobagent prompt list
jobagent prompt show <id>
jobagent prompt set <id>

jobagent config show
jobagent config set

jobagent history
jobagent data delete
jobagent logs
```

Commands may evolve. Avoid implementing commands that do not have a product purpose.
