# Feature: Application Answers

## Question categories

1. Deterministic profile facts
2. Derived factual answers
3. Generative narrative answers
4. Unknown/unsafe questions requiring user input

## Resolver order

Question
→ exact profile answer
→ deterministic derivation
→ generated answer
→ ask user if confidence/grounding is insufficient

## Acceptance Criteria

### AC-001
Known factual questions use profile data.

### AC-002
Generative questions use job and candidate context.

### AC-003
Unknown factual questions are not guessed.

### AC-004
Generated answers are editable.

### AC-005
Answers are associated with the job/application.
