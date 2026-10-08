# JobPilot Master Specification

This is the documentation map, not a replacement for the linked requirements.
For implementation work, follow the source-of-truth order in [SETUP](../.agents/SETUP.md): current user instructions, accepted ADRs, requirements, architecture, feature specs, agent guidance, then existing code.

## Start here

- [Product overview](00_product/00_overview.md): purpose and boundaries.
- [Current state](00_product/current_state.md): implemented capabilities, verification gaps and next work.
- [Roadmap](00_product/roadmap.md): original phase scope.
- [CLI reference](CLI.md): implemented commands and examples.

## Requirements and behavior

- [Product requirements](00_product/01_requirements.md) and [global acceptance criteria](00_product/acceptance_criteria.md).
- [User flows](00_product/03_user_flows.md) and [application modes](00_product/04_application_modes.md).
- [Feature specifications](02_features/00_index.md).
- [Local workspaces](02_features/21_local_workspaces.md): isolated profiles in one local installation.
- [Edge cases](00_product/05_edge_cases.md) and [non-goals](00_product/06_non_goals.md).

## Architecture and decisions

- [Architecture overview](01_architecture/00_overview.md) and [agent workflows](03_agent/00_overview.md).
- [Browser architecture](06_browser/00_browser_architecture.md).
- [Connector contract](04_connectors/00_connector_sdk.md) and [plugin proposals](05_plugins/00_plugin_architecture.md).
- [Security](07_security/00_threat_model.md) and [privacy](07_security/01_privacy.md).
- [Architecture decision records](09_decisions/): check each record's status and amendments; historical context is not current implementation status.

## Development and release

- [Definition of Done](08_development/definition_of_done.md) and [testing](08_development/testing.md).
- [Hot reload](08_development/hot_reload.md) and [desktop setup](../desktop/README.md).
- [Packaging and releases](08_development/packaging.md).
