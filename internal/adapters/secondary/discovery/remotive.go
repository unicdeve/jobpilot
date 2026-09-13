package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// DefaultRemotiveBaseURL is Remotive's public, no-auth remote-jobs API. See
// https://remotive.com/api-documentation.
const DefaultRemotiveBaseURL = "https://remotive.com/api/remote-jobs"

// remotiveCacheTTL is deliberately conservative: Remotive's own API terms
// say results are delayed 24h and ask for at most ~4 requests/day.
const remotiveCacheTTL = 4 * time.Hour

// remotivePublicationDateLayout is the format Remotive's API uses for
// publication_date ("2026-09-07T01:10:43", no timezone offset).
const remotivePublicationDateLayout = "2006-01-02T15:04:05"

// RemotiveSource is a ports.JobSource backed by Remotive's public API. It
// only supports search: Remotive has no endpoint to fetch an arbitrary job
// by URL.
type RemotiveSource struct {
	baseURL    string
	httpClient *http.Client

	mu         sync.Mutex
	cachedAt   time.Time
	cachedJobs []domain.Job
}

// NewRemotiveSource constructs a RemotiveSource targeting baseURL.
func NewRemotiveSource(baseURL string) *RemotiveSource {
	return &RemotiveSource{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// ID returns the source's unique identifier.
func (s *RemotiveSource) ID() string { return "remotive" }

// Name returns the source's human-readable name.
func (s *RemotiveSource) Name() string { return "Remotive" }

// Capabilities reports that Remotive supports search but not
// lookup-by-arbitrary-URL.
func (s *RemotiveSource) Capabilities() ports.SourceCapabilities {
	return ports.SourceCapabilities{CanSearch: true, FiltersServerSide: false, CanGetJob: false}
}

type remotiveResponse struct {
	Jobs []remotiveJob `json:"jobs"`
}

type remotiveJob struct {
	ID                        int      `json:"id"`
	URL                       string   `json:"url"`
	Title                     string   `json:"title"`
	CompanyName               string   `json:"company_name"`
	Tags                      []string `json:"tags"`
	JobType                   string   `json:"job_type"`
	PublicationDate           string   `json:"publication_date"`
	CandidateRequiredLocation string   `json:"candidate_required_location"`
	Description               string   `json:"description"`
}

// Search returns Remotive's current job listing (cached per
// remotiveCacheTTL). The criteria are ignored here on purpose: Remotive's
// API accepts search/limit/category parameters but acts on none of them, so
// the listing is returned whole and JobDiscoveryService applies the criteria
// — see FiltersServerSide in ports.SourceCapabilities.
func (s *RemotiveSource) Search(ctx context.Context, _ domain.SearchCriteria) ([]domain.Job, error) {
	return s.fetch(ctx)
}

// GetJob always fails: Remotive has no API to fetch a single job by URL.
func (s *RemotiveSource) GetJob(_ context.Context, url string) (*domain.Job, error) {
	return nil, fmt.Errorf("remotive does not support fetching a job by URL: %s", url)
}

func (s *RemotiveSource) fetch(ctx context.Context) ([]domain.Job, error) {
	s.mu.Lock()
	if time.Since(s.cachedAt) < remotiveCacheTTL && s.cachedJobs != nil {
		jobs := s.cachedJobs
		s.mu.Unlock()
		return jobs, nil
	}
	s.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach remotive: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remotive returned non-OK status: %d", resp.StatusCode)
	}

	var parsed remotiveResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode remotive response: %w", err)
	}

	jobs := make([]domain.Job, 0, len(parsed.Jobs))
	for _, item := range parsed.Jobs {
		jobs = append(jobs, s.toDomainJob(item))
	}

	s.mu.Lock()
	s.cachedJobs = jobs
	s.cachedAt = time.Now()
	s.mu.Unlock()

	return jobs, nil
}

func (s *RemotiveSource) toDomainJob(item remotiveJob) domain.Job {
	description, err := htmlToMarkdown(item.Description)
	if err != nil {
		description = item.Description
	}

	var locations []string
	if item.CandidateRequiredLocation != "" {
		locations = []string{item.CandidateRequiredLocation}
	}

	postedAt, _ := time.Parse(remotivePublicationDateLayout, item.PublicationDate)

	raw, _ := json.Marshal(item)

	return domain.Job{
		CanonicalURL:        item.URL,
		SourceID:            s.ID(),
		SourceJobID:         strconv.Itoa(item.ID),
		Title:               item.Title,
		CompanyName:         item.CompanyName,
		DescriptionMarkdown: description,
		Locations:           locations,
		RemoteType:          "remote",
		EmploymentType:      item.JobType,
		Skills:              item.Tags,
		PostedAt:            postedAt,
		DiscoveredAt:        time.Now(),
		RawSourceReference:  string(raw),
	}
}
