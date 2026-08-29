package domain

import "time"

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
}

// SearchCriteria defines the parameters for finding jobs.
type SearchCriteria struct {
	Keywords          string   `json:"keywords"`
	Title             string   `json:"title"`
	Location          string   `json:"location"`
	RemotePreference  string   `json:"remote_preference"`
	EmploymentType    string   `json:"employment_type"`
	SalaryMin         float64  `json:"salary_min"`
	SalaryMax         float64  `json:"salary_max"`
	ExperienceMin     int      `json:"experience_min"`
	ExperienceMax     int      `json:"experience_max"`
	IncludedSources   []string `json:"included_sources"`
	ExcludedCompanies []string `json:"excluded_companies"`
	CustomURLs        []string `json:"custom_urls"`
}
