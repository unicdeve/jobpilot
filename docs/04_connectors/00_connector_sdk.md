# Connector Architecture

## Purpose

Provide a common interface for job discovery and, later, application integration.

## Job source interface

Conceptually:

```go
type JobSource interface {
    ID() string
    Name() string
    Capabilities() Capabilities
    Search(ctx context.Context, criteria SearchCriteria) ([]Job, error)
    GetJob(ctx context.Context, url string) (*Job, error)
}
```

The final API should be idiomatic Go and versioned when external SDK support begins.

## Capabilities

Possible capabilities:
- search
- get job
- authenticated search
- application form discovery
- application form filling
- submission

A connector must declare only capabilities it actually supports.

## Initial strategy

Prioritize stable/public sources and ATS platforms rather than making scraping the foundation.
