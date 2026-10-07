# Policy Engine

Policy rules are deterministic.

Implemented Scout eligibility combines saved-search filters with a match-score floor and per-run ceiling.
Other examples below are requirements or possible future rules, not a general implemented policy engine.
No policy permits automatic submission (ADR-0014).

Examples:
- minimum match score
- excluded companies
- location restrictions
- employment type restrictions
- salary thresholds
- maximum applications per period
- required approval
- forbidden question categories

The LLM may provide recommendations, but the policy engine makes the final authorization decision.
