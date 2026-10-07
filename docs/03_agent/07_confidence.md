# Confidence

Confidence should be explicit for AI-generated decisions.

The following is proposed guidance, not a shared runtime policy.
Current scoring and answer resolution use their documented service-specific validation rules.
A confidence value never authorizes submission or overrides deterministic rules.

Suggested behavior:

- High confidence: proceed according to policy.
- Medium confidence: request review where appropriate.
- Low confidence: stop and ask the user.

Confidence must not be treated as a mathematical guarantee. It is a workflow signal.
