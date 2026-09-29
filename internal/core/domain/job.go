package domain

import (
	"strings"
	"time"
)

// Job represents a normalized job posting from any source.
type Job struct {
	ID                  string    `json:"id"`
	CanonicalURL        string    `json:"canonical_url"`
	SourceID            string    `json:"source_id"`
	SourceJobID         string    `json:"source_job_id"`
	Title               string    `json:"title"`
	CompanyName         string    `json:"company_name"`
	CompanyURL          string    `json:"company_url"`
	DescriptionMarkdown string    `json:"description_markdown"`
	Locations           []string  `json:"locations"`
	RemoteType          string    `json:"remote_type"`     // e.g., "remote", "hybrid", "onsite"
	EmploymentType      string    `json:"employment_type"` // e.g., "full-time", "contract"
	SalaryMin           float64   `json:"salary_min"`
	SalaryMax           float64   `json:"salary_max"`
	SalaryCurrency      string    `json:"salary_currency"`
	SalaryPeriod        string    `json:"salary_period"` // e.g., "yearly", "hourly"
	ExperienceMin       int       `json:"experience_min"`
	ExperienceMax       int       `json:"experience_max"`
	Skills              []string  `json:"skills"`
	PostedAt            time.Time `json:"posted_at"`
	ExpiresAt           time.Time `json:"expires_at"`
	DiscoveredAt        time.Time `json:"discovered_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	RawSourceReference  string    `json:"raw_source_reference"`
	// NotInterestedAt is when the user ruled the job out, or nil. A job
	// ruled out stays stored, leaves the lists and a search's result, and
	// is passed over by scouts (docs/02_features/15_data_deletion.md).
	NotInterestedAt *time.Time `json:"not_interested_at,omitempty"`
}

// SearchCriteria defines the parameters for finding jobs.
//
// Every field here is honoured by Matches. Fields that no source could
// populate were removed rather than left to be accepted and silently
// ignored: years-of-experience bounds (no connector reports them) and a
// source allowlist (superseded by only registering the sources in use).
type SearchCriteria struct {
	Keywords          string   `json:"keywords"`
	Title             string   `json:"title"`
	Location          string   `json:"location"`
	RemotePreference  string   `json:"remote_preference"`
	EmploymentType    string   `json:"employment_type"`
	SalaryMin         float64  `json:"salary_min"`
	SalaryMax         float64  `json:"salary_max"`
	ExcludedCompanies []string `json:"excluded_companies"`
}

// IsEmpty reports whether the criteria would match every job in existence.
//
// Discovery refuses to run on empty criteria. Both public sources ignore
// their own query parameters, so an unfiltered search means importing every
// posting on the board — hundreds of jobs the user never asked about, mixed
// permanently into their local history.
func (c SearchCriteria) IsEmpty() bool {
	return strings.TrimSpace(c.Keywords) == "" &&
		strings.TrimSpace(c.Title) == "" &&
		strings.TrimSpace(c.Location) == "" &&
		strings.TrimSpace(c.RemotePreference) == "" &&
		strings.TrimSpace(c.EmploymentType) == "" &&
		c.SalaryMin == 0 && c.SalaryMax == 0 &&
		len(c.ExcludedCompanies) == 0
}

// Matches reports whether job satisfies every criterion that is set.
//
// This is the one definition of what a search means. It is applied by
// JobDiscoveryService to results from any source that cannot filter
// server-side, so a connector cannot forget to filter or disagree about the
// rule; SQLite's stored-job search applies the same term semantics.
//
// Unset criteria never exclude anything, and neither does missing data on
// the job: a salary bound skips postings whose salary is unknown rather than
// discarding them, since most boards publish no salary at all and treating
// absence as a mismatch would hide nearly everything.
func (c SearchCriteria) Matches(job Job) bool {
	if !matchesAllTerms(c.Keywords, job.Title+" "+job.DescriptionMarkdown) {
		return false
	}
	if !matchesAllTerms(c.Title, job.Title) {
		return false
	}
	if !matchesAllTerms(c.Location, strings.Join(job.Locations, " ")) {
		return false
	}
	if !equalsFold(c.RemotePreference, job.RemoteType) {
		return false
	}
	if !equalsFold(c.EmploymentType, job.EmploymentType) {
		return false
	}
	if !withinSalary(c, job) {
		return false
	}

	for _, excluded := range c.ExcludedCompanies {
		if strings.EqualFold(strings.TrimSpace(excluded), strings.TrimSpace(job.CompanyName)) {
			return false
		}
	}
	return true
}

// SearchTerms splits a criteria field into the individual words a job must
// contain. Matching on terms rather than the raw string is what lets
// "backend engineer" find a posting titled "Engineer, Backend (Go)" —
// a substring test misses it, and job titles order their words freely.
func SearchTerms(field string) []string {
	return strings.Fields(strings.ToLower(strings.TrimSpace(field)))
}

// matchesAllTerms reports whether text contains every term in field. An
// empty field matches everything, since an unset criterion excludes nothing.
func matchesAllTerms(field, text string) bool {
	haystack := strings.ToLower(text)
	for _, term := range SearchTerms(field) {
		if !strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}

// equalsFold compares a criterion against a job field, treating an unset
// criterion as "no preference".
func equalsFold(criterion, value string) bool {
	criterion = strings.TrimSpace(criterion)
	return criterion == "" || strings.EqualFold(criterion, strings.TrimSpace(value))
}

// withinSalary applies the salary bounds on a best-effort basis. Salary data
// is sparse and inconsistently reported across boards — Arbeitnow publishes
// none at all — so a job with no salary recorded is kept rather than judged
// against a bound there is no data for.
func withinSalary(c SearchCriteria, job Job) bool {
	if job.SalaryMin == 0 && job.SalaryMax == 0 {
		return true
	}
	// The job's advertised top end must reach the floor the user asked for,
	// and its bottom end must not exceed their ceiling.
	if c.SalaryMin > 0 && max(job.SalaryMin, job.SalaryMax) < c.SalaryMin {
		return false
	}
	if c.SalaryMax > 0 && job.SalaryMin > c.SalaryMax {
		return false
	}
	return true
}

// SavedSearch is a named set of criteria the user keeps and re-runs, so that
// discovery is always driven by something they chose. See
// docs/02_features/04_job_discovery.md AC-001.
type SavedSearch struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Criteria  SearchCriteria `json:"criteria"`
	IsDefault bool           `json:"is_default"`
	// LastRunAt is when the search last ran against the job sources and at
	// least one answered, from any path: Discover, setup, the CLI or a
	// scout. Nil is a search that never has.
	LastRunAt *time.Time `json:"last_run_at"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// SearchRefreshInterval is how old a search's last run must be before the
// desktop runs it again on opening (docs/02_features/04_job_discovery.md
// AC-009). Opening the app several times a day asks the sources once in
// that time, not every time.
const SearchRefreshInterval = 6 * time.Hour

// Stale reports whether the search is due to run again at now: it never
// has, or its last run is at least SearchRefreshInterval old.
func (s *SavedSearch) Stale(now time.Time) bool {
	return s.LastRunAt == nil || now.Sub(*s.LastRunAt) >= SearchRefreshInterval
}

// Remote types a job can have. A source that does not say leaves it empty.
const (
	RemoteTypeRemote = "remote"
	RemoteTypeHybrid = "hybrid"
	RemoteTypeOnsite = "onsite"
)

// ValidRemoteType reports whether value is one of the remote types.
func ValidRemoteType(value string) bool {
	switch value {
	case RemoteTypeRemote, RemoteTypeHybrid, RemoteTypeOnsite:
		return true
	}
	return false
}

// JobFilter narrows a listing of stored jobs. Every field is optional, and
// an unset one excludes nothing.
type JobFilter struct {
	// Keywords must each appear in the title or the company name.
	Keywords string
	// SourceID keeps only the jobs from one source, e.g. "arbeitnow".
	SourceID string
	// RemoteType keeps only the jobs of one remote type.
	RemoteType string
	// PostedSince keeps the jobs posted at or after it. A job whose source
	// gave no posting date is judged by when JobPilot found it, which is
	// the same date the job list shows for it.
	PostedSince time.Time
	// Interest says whether jobs the user ruled out are included. The zero
	// value leaves them out, as every list the user browses does.
	Interest JobInterest
}

// JobInterest selects jobs by whether the user marked them not interested.
type JobInterest string

// The jobs a listing includes.
const (
	// JobsInterested leaves out the jobs marked not interested. It is the
	// zero value, so a listing that says nothing hides them.
	JobsInterested JobInterest = ""
	// JobsNotInterested keeps only the jobs marked not interested.
	JobsNotInterested JobInterest = "not_interested"
	// JobsAll includes both. A scout reads jobs this way, because it must
	// see a job it passes over to record why.
	JobsAll JobInterest = "all"
)

// Valid reports whether i is a known selection.
func (i JobInterest) Valid() bool {
	switch i {
	case JobsInterested, JobsNotInterested, JobsAll:
		return true
	}
	return false
}

// JobPage is one page of stored jobs together with the total number of jobs
// matching the same filter, so a caller paginating through history can tell
// whether more pages remain without fetching them.
type JobPage struct {
	Jobs  []*Job `json:"jobs"`
	Total int    `json:"total"`
}
