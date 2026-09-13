package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// DefaultArbeitnowBaseURL is Arbeitnow's public, no-auth job board API.
// See https://www.arbeitnow.com/blog/job-board-api.
const DefaultArbeitnowBaseURL = "https://www.arbeitnow.com/api/job-board-api"

// arbeitnowCacheTTL matches Arbeitnow's own stated update cadence ("jobs are
// updated every hour"), so repeated searches don't poll the API faster than
// its data actually changes.
const arbeitnowCacheTTL = 15 * time.Minute

// ArbeitnowSource is a ports.JobSource backed by Arbeitnow's public job
// board API. It only supports search: Arbeitnow has no endpoint to fetch an
// arbitrary job by URL.
type ArbeitnowSource struct {
	baseURL    string
	httpClient *http.Client

	mu         sync.Mutex
	cachedAt   time.Time
	cachedJobs []domain.Job
}

// NewArbeitnowSource constructs an ArbeitnowSource targeting baseURL.
func NewArbeitnowSource(baseURL string) *ArbeitnowSource {
	return &ArbeitnowSource{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// ID returns the source's unique identifier.
func (s *ArbeitnowSource) ID() string { return "arbeitnow" }

// Name returns the source's human-readable name.
func (s *ArbeitnowSource) Name() string { return "Arbeitnow" }

// Capabilities reports that Arbeitnow supports search but not
// lookup-by-arbitrary-URL.
func (s *ArbeitnowSource) Capabilities() ports.SourceCapabilities {
	return ports.SourceCapabilities{CanSearch: true, FiltersServerSide: false, CanGetJob: false}
}

type arbeitnowResponse struct {
	Data []arbeitnowJob `json:"data"`
}

type arbeitnowJob struct {
	Slug        string   `json:"slug"`
	CompanyName string   `json:"company_name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Remote      bool     `json:"remote"`
	URL         string   `json:"url"`
	Tags        []string `json:"tags"`
	JobTypes    []string `json:"job_types"`
	Location    string   `json:"location"`
	CreatedAt   int64    `json:"created_at"`
}

// Search returns Arbeitnow's current job listing (page 1, cached per
// arbeitnowCacheTTL). The criteria are ignored here on purpose: Arbeitnow's
// API accepts query parameters but acts on none of them, so the listing is
// returned whole and JobDiscoveryService applies the criteria — see
// FiltersServerSide in ports.SourceCapabilities.
func (s *ArbeitnowSource) Search(ctx context.Context, _ domain.SearchCriteria) ([]domain.Job, error) {
	return s.fetch(ctx)
}

// GetJob always fails: Arbeitnow has no API to fetch a single job by URL.
func (s *ArbeitnowSource) GetJob(_ context.Context, url string) (*domain.Job, error) {
	return nil, fmt.Errorf("arbeitnow does not support fetching a job by URL: %s", url)
}

func (s *ArbeitnowSource) fetch(ctx context.Context) ([]domain.Job, error) {
	s.mu.Lock()
	if time.Since(s.cachedAt) < arbeitnowCacheTTL && s.cachedJobs != nil {
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
		return nil, fmt.Errorf("failed to reach arbeitnow: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("arbeitnow returned non-OK status: %d", resp.StatusCode)
	}

	var parsed arbeitnowResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode arbeitnow response: %w", err)
	}

	jobs := make([]domain.Job, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		jobs = append(jobs, s.toDomainJob(item))
	}

	s.mu.Lock()
	s.cachedJobs = jobs
	s.cachedAt = time.Now()
	s.mu.Unlock()

	return jobs, nil
}

func (s *ArbeitnowSource) toDomainJob(item arbeitnowJob) domain.Job {
	description, err := htmlToMarkdown(item.Description)
	if err != nil {
		description = item.Description
	}

	remoteType := "onsite"
	if item.Remote {
		remoteType = "remote"
	}

	var locations []string
	if item.Location != "" {
		for loc := range strings.SplitSeq(item.Location, ";") {
			if trimmed := strings.TrimSpace(loc); trimmed != "" {
				locations = append(locations, trimmed)
			}
		}
	}

	raw, _ := json.Marshal(item)

	return domain.Job{
		CanonicalURL:        item.URL,
		SourceID:            s.ID(),
		SourceJobID:         item.Slug,
		Title:               item.Title,
		CompanyName:         item.CompanyName,
		DescriptionMarkdown: description,
		Locations:           locations,
		RemoteType:          remoteType,
		EmploymentType:      strings.Join(item.JobTypes, ", "),
		Skills:              item.Tags,
		PostedAt:            time.Unix(item.CreatedAt, 0),
		DiscoveredAt:        time.Now(),
		RawSourceReference:  string(raw),
	}
}
