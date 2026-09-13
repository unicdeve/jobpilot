# Connector Architecture

## Purpose

Provide a common interface for job discovery and, later, application integration.

## Job source interface

Conceptually:

```go
type JobSource interface {
    ID() string
    Name() string
    Capabilities() SourceCapabilities
    Search(ctx context.Context, criteria SearchCriteria) ([]Job, error)
    GetJob(ctx context.Context, url string) (*Job, error)
}
```

The final API should be idiomatic Go and versioned when external SDK support begins.

## Capabilities

Possible capabilities:
- search
- server-side filtering
- get job
- authenticated search
- application form discovery
- application form filling
- submission

A connector must declare only capabilities it actually supports.

### Filtering is the discovery service's guarantee

`Search` receives the full `SearchCriteria`, but a connector is not required
to apply it. Most public boards cannot: Arbeitnow and Remotive both accept
query parameters and act on none of them, returning their entire listing
whatever is asked for.

A connector therefore declares `FiltersServerSide`:

- **false** — return whatever the source gives you, unfiltered.
  `JobDiscoveryService` applies `SearchCriteria.Matches` to the results
  before any of them are stored.
- **true** — the source genuinely honoured the criteria. Its results are
  taken as-is, because it knows more about its own data than a local
  substring rule does.

Filtering lives in the service rather than in each connector so that the
matching rule is defined once, and so that a new connector cannot import an
entire job board by forgetting to filter. This was not always so: the same
filter loop was previously copy-pasted into every connector, sharing a
helper that lived in the mock source.

Note the distinction between the two flags. `CanSearch: false` means the
source is skipped entirely and contributes nothing — it does not mean "can
search but cannot filter", which is `FiltersServerSide: false`.

## Initial strategy

Prioritize stable/public sources and ATS platforms rather than making scraping the foundation.
