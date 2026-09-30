package domain

// SourceStatus is how one job source answered a discovery run.
type SourceStatus string

const (
	// SourceStatusOK means the source answered with its jobs.
	SourceStatusOK SourceStatus = "ok"
	// SourceStatusUnreachable means the source gave no answer at all, which
	// is what being offline looks like.
	SourceStatusUnreachable SourceStatus = "unreachable"
	// SourceStatusFailed means the source answered, but not with jobs: an
	// error status, or a response that could not be read.
	SourceStatusFailed SourceStatus = "failed"
)

// SourceInfo names a job source a discovery run asks, so a client can say
// what a search is waiting on before it answers.
type SourceInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SourceOutcome is what one source contributed to a discovery run.
//
// It carries no error text. A transport error names the URL it requested,
// and a source that needs a key can carry that key in its query string, so
// the detail is logged by the daemon and never returned to a client.
type SourceOutcome struct {
	SourceID   string       `json:"source_id"`
	SourceName string       `json:"source_name"`
	Status     SourceStatus `json:"status"`
	// Found is how many of the source's jobs matched the criteria, counting
	// ones already stored. It is zero for a source that did not answer.
	Found int `json:"found"`
	// Failing names the parts of a source that did not answer, such as a
	// watched company whose board has gone (ADR-0024 §5).
	Failing []string `json:"failing,omitempty"`
}

// DiscoveryResult is the outcome of searching the job sources: the jobs that
// matched, how many of them were not stored before, and how each source
// answered, so that a thin result can be told apart from a source that was
// not heard from.
type DiscoveryResult struct {
	Jobs []*Job `json:"jobs"`
	// New counts the jobs this run stored for the first time. The rest were
	// already known and are returned as stored.
	New     int             `json:"new"`
	Sources []SourceOutcome `json:"sources"`
}

// Unanswered returns the sources that did not answer with jobs, in the
// order they were asked.
func (r *DiscoveryResult) Unanswered() []SourceOutcome {
	var unanswered []SourceOutcome
	for _, source := range r.Sources {
		if source.Status != SourceStatusOK {
			unanswered = append(unanswered, source)
		}
	}
	return unanswered
}
