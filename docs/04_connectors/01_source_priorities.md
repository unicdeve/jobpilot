# Connector Source Priorities

## Implemented discovery

Fresh installations can discover jobs without source credentials.
The daemon registers seven public job sources and a company-pages source:

| Source | Coverage |
| --- | --- |
| Arbeitnow | European roles, including many German listings |
| Remotive | Remote roles by category |
| RemoteOK | Remote roles from its public feed |
| Himalayas | Remote roles searched by title, keywords and country |
| Hacker News, Who is hiring? | Hiring-team postings from the monthly thread |
| We Work Remotely | Remote roles with employer-supplied country restrictions |
| Jobicy | Remote roles from worldwide and country/region feeds |
| Company pages | Openings at companies the candidate watches |

Watched-company providers are Greenhouse, Lever, Ashby, Workable and SmartRecruiters (ADR-0024, ADR-0026).
The company-pages source returns nothing until the user watches a company.
Watching checks the board but does not import jobs; discovery runs the saved criteria against its openings.

Individual job URLs can also be imported.
If no connector can retrieve the URL, the browser worker reads the rendered posting and the model extracts job data as untrusted content.
See [Job discovery](../02_features/04_job_discovery.md) and [Company watchlists](../02_features/17_company_watchlists.md).

## Deferred

- Keyed sources: Adzuna is the next candidate once a key is available to implement and measure it; USAJOBS and Jooble are not built.
  Credential storage exists (ADR-0019), but choosing between multiple labels for a source's runs remains undecided.
- Brave Search API: deferred alternative to Google's closed Programmable Search API (ADR-0020).
- Authenticated discovery sites and browser-based discovery connectors.
- User-installed third-party connectors and their external SDK.

Google search scraping is not a fallback: JobPilot does not bypass anti-bot gates (ADR-0013, ADR-0014).
Working Nomads remains excluded until its terms permit access (ADR-0023).
Public APIs, feeds and ATS endpoints remain the preferred foundation.

## Access rule

Do not assume a site permits automated access.
Respect its terms, technical restrictions and user-controlled sessions.
Discovery connectors never submit applications.
