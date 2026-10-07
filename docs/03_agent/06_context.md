# Context Management

Services construct task-specific prompts rather than a universal context pipeline.
Prompt customization separates editable guidance from a code-owned safety and response frame (ADR-0031).
The frame follows guidance in the system prompt and takes precedence; job content remains untrusted task input.

Conceptual context inputs:

1. System instructions
2. Task instructions
3. Candidate structured facts
4. Relevant resume content
5. Job data
6. Application question
7. User prompt guidance (in the system prompt, separate from untrusted job data)

Avoid sending the entire local database or unrelated history.

Prefer structured context and targeted document retrieval.
