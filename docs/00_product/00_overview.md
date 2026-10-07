# Product Overview

## Name

JobPilot

## Vision

An open-source, local-first assistant that helps people discover relevant jobs, evaluate opportunities against their profile and resume, and prepare tailored applications for review.
JobPilot can fill an application form, but it always leaves submission to the user ([ADR-0014](../09_decisions/0014-scouts-and-no-unattended-submission.md)).

## Product positioning

JobPilot is not primarily a scraper. Job discovery is a pluggable capability. The product is an agent runtime specialized for job hunting and applications.

## Core value

- Reduce time spent searching for relevant jobs.
- Improve job/profile matching.
- Reduce repetitive application writing.
- Keep candidate data local by default.
- Give users transparent control over AI-generated work.

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

Review mode is the default application mode.

## Future direction

The architecture should support third-party connectors, tools, and plugins without requiring the core application to be rewritten.
The extension SDK and plugin runtime are post-MVP proposals, not installed capabilities.
See [current state](current_state.md) for implemented features and remaining work.
