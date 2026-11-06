# Application Form Filling - removed

JobPilot filled application forms from profile facts and generated answers until [ADR-0034](../09_decisions/0034-the-candidate-writes-and-fills-their-own-application.md) removed browser preparation.
It no longer types into an employer's form.
The deterministic resolver that chose a field's value now suggests an answer to a question the candidate noted, which they use only if they choose to ([Application answers](../02_features/07_application_answers.md)).
