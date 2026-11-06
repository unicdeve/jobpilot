# Product Overview

## Name

JobPilot

## Vision

An open-source, local-first assistant that helps people discover relevant jobs, evaluate opportunities against their profile and resume, and keep track of the applications they write and send themselves.
JobPilot never writes application content and never fills in or submits an employer's form ([ADR-0034](../09_decisions/0034-the-candidate-writes-and-fills-their-own-application.md)).

## Product positioning

JobPilot is not primarily a scraper. Job discovery is a pluggable capability. The product is an agent runtime specialized for job hunting and applications.

## Core value

- Reduce time spent searching for relevant jobs.
- Improve job/profile matching.
- Reduce repetitive application work: details ready to copy, and answers written once and reused by the candidate.
- Keep candidate data local by default.
- Keep every word an employer reads the candidate's own.

## Local-first model

The application can operate without a hosted account or mandatory backend. Local data remains on the user's machine.

Internet is required for:
- discovering new jobs
- fetching remote job data
- external integrations
- using a configured hosted LLM

Desktop notifications are implemented; email notifications are deferred.

Existing local history can be viewed and managed offline.

## Platforms

Target:
- macOS
- Linux
- Windows

## Interfaces

- CLI
- Native-feeling desktop application

## Default behavior

The candidate applies on the employer's site in their own browser; JobPilot opens the page and keeps their details, letter and answers beside it ([ADR-0034](../09_decisions/0034-the-candidate-writes-and-fills-their-own-application.md)).

## Future direction

The architecture should support third-party connectors, tools, and plugins without requiring the core application to be rewritten.
The extension SDK and plugin runtime are post-MVP proposals, not installed capabilities.
See [current state](current_state.md) for implemented features and remaining work.
