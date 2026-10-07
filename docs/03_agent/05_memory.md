# Agent Memory

Application questions and answers, versioned cover letters and state transitions are persisted locally.
Personal answers are explicit candidate-wide or application-specific records that users can inspect and edit (ADR-0022).
The answer generator uses relevant personal-answer material as context for new content.

This is application history and user-owned data, not opaque model memory.
Broader automatic retrieval of historical answers is not implemented.
See [Application answers](../02_features/07_application_answers.md) for the resolver contract.
