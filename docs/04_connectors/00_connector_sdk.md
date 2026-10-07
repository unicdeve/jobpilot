# Connector Architecture

## Scope and interface

Built-in connectors provide job discovery through [`ports.JobSource`](../../internal/core/ports/discovery.go).
Browser form preparation is a separate worker; connectors have no submission capability.
JobPilot never submits applications (ADR-0014).
An external connector SDK and versioned plugin contract remain deferred.

```go
type JobSource interface {
    ID() string
    Name() string
    Capabilities() SourceCapabilities
    Search(ctx context.Context, criteria domain.SearchCriteria) ([]domain.Job, error)
    GetJob(ctx context.Context, url string) (*domain.Job, error)
}
```

`SourceCapabilities` contains `CanSearch`, `FiltersServerSide`, `CanGetJob` and `CanAuthenticate`.
A connector declares only what it supports.
Discovery skips sources whose `CanSearch` is false.

## Filtering

When `FiltersServerSide` is false, the connector returns the source's results and `JobDiscoveryService` applies `SearchCriteria.Matches` before storing them.
This is the current public sources' contract, including sources that use query parameters to narrow their fetched data.
Set `FiltersServerSide` true only when the source genuinely honours the complete criteria; the service then accepts its results as-is.
Keeping filtering in core gives every connector the same deterministic matching rule.

## Locations

A connector writes the source's own words into `Job.Locations`; core interprets them (ADR-0023).
For a remote job, these describe where the candidate may work; for an onsite or hybrid job, where the office is.

- Himalayas' empty `locationRestrictions` means worldwide and is stored as `Worldwide`, rather than as an unstated location.
- We Work Remotely's employer-supplied `country` takes precedence over its broader `region` label.
- Ashby's `address` and Lever's `country` name offices or hiring entities and are omitted when they do not describe where the candidate may work (ADR-0024).

## Failures and partial answers

A source that gives no answer—connection failure, name-resolution failure or timeout—wraps its HTTP transport error with `ports.ErrSourceUnreachable`.
An error status or unreadable response body is an ordinary source failure: the source was reached.
Discovery asks sources concurrently and treats a connector panic as a source failure.

When all sources are unreachable, discovery returns `ErrSourcesUnreachable` and explains that fresh discovery needs internet access.
When all fail but at least one was reached, discovery reports source failure instead.
Successful sources still contribute jobs when another fails.

Connector error text is never returned to clients: a transport error can expose a URL containing a key.
Clients receive source names and statuses.
Logs must still avoid exposing secrets.

A source composed of several boards can return `ports.PartialAnswer` alongside successful results.
`Failing` identifies boards that did not answer; `Err` is nil if another board answered, or carries the overall failure otherwise.

## Watched company boards

[`ports.BoardReader`](../../internal/core/ports/discovery.go) reads boards independently of who watches them.
The company-pages discovery service coordinates the watched companies, applies criteria and reports partial failures.
Supported providers and deferred sources are listed in [Source priorities](01_source_priorities.md).
See [Company watchlists](../02_features/17_company_watchlists.md), ADR-0024 and ADR-0026 for board-address resolution, caching, location interpretation and description fetching.
