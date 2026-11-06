# Feature: Review Mode - removed

Review mode was removed by [ADR-0034](../09_decisions/0034-the-candidate-writes-and-fills-their-own-application.md).

It filled an employer's application form in a browser JobPilot controlled, from profile facts and generated answers, and left the window open for the user to check and submit.
Applications that read as written by AI were being rejected, and filling forms carried most of the product's fragility for little that copying a profile fact does not do.

What replaced it is in [Applications](08_applications.md): the candidate opens the application page in their own browser, copies their details from the application page, writes their own letter and answers, and records when they sent it.

What stays from Review mode:

- JobPilot never submits an application (ADR-0014).
- Marking an application ready to send, and recording that it was sent, are the user's explicit actions, kept in its history.
