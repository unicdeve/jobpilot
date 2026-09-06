# Agent Architecture Overview

The JobPilot uses an orchestrated tool-calling architecture rather than a large collection of autonomous agents.

```text
Observe
→ Plan
→ Select tool
→ Execute
→ Validate result
→ Persist event
→ Continue or finish
```

The LLM proposes actions. The runtime validates and executes them.

The LLM never receives unrestricted system access.
