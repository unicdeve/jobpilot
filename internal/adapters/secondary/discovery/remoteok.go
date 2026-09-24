package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// DefaultRemoteOKBaseURL is RemoteOK's public, no-auth job feed.
// See https://remoteok.com/api.
const DefaultRemoteOKBaseURL = "https://remoteok.com/api"

// remoteOKCacheTTL is conservative: the feed is a single document covering
// the whole board, and RemoteOK asks API users not to hammer it.
const remoteOKCacheTTL = 30 * time.Minute

// RemoteOKSource is a ports.JobSource backed by RemoteOK's public feed.
//
// RemoteOK's API terms ask that consumers link back and name RemoteOK as
// the source. Every job keeps its RemoteOK URL as its canonical URL and
// carries this source's ID, which the desktop and CLI both show, so a job
// found here is always attributed and always clickable.
type RemoteOKSource struct {
	baseURL    string
	httpClient *http.Client
	cache      *jobCache[domain.Job]
}

// NewRemoteOKSource constructs a RemoteOKSource targeting baseURL.
func NewRemoteOKSource(baseURL string) *RemoteOKSource {
	return &RemoteOKSource{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 20 * time.Second},
		cache:      newJobCache[domain.Job](remoteOKCacheTTL),
	}
}

// ID returns the source's unique identifier.
func (s *RemoteOKSource) ID() string { return "remoteok" }

// Name returns the source's human-readable name.
func (s *RemoteOKSource) Name() string { return "RemoteOK" }

// Capabilities reports search only: the feed has no by-URL lookup.
func (s *RemoteOKSource) Capabilities() ports.SourceCapabilities {
	return ports.SourceCapabilities{CanSearch: true, FiltersServerSide: false, CanGetJob: false}
}

// remoteOKEntry is one element of the feed.
//
// The first element is not a job at all: it is a metadata object carrying
// the feed's legal notice and last-updated time. Entries are therefore
// identified by having a position, not by their position in the array — a
// feed that one day stops leading with the notice should not silently turn
// it into a job called "".
type remoteOKEntry struct {
	ID          string   `json:"id"`
	Slug        string   `json:"slug"`
	Position    string   `json:"position"`
	Company     string   `json:"company"`
	Description string   `json:"description"`
	Location    string   `json:"location"`
	Tags        []string `json:"tags"`
	URL         string   `json:"url"`
	ApplyURL    string   `json:"apply_url"`
	Date        string   `json:"date"`
	SalaryMin   float64  `json:"salary_min"`
	SalaryMax   float64  `json:"salary_max"`
	Legal       string   `json:"legal"`
}

// Search returns the whole feed; JobDiscoveryService applies the criteria.
func (s *RemoteOKSource) Search(ctx context.Context, _ domain.SearchCriteria) ([]domain.Job, error) {
	if cached, fresh := s.cache.get(); fresh {
		return cached, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// RemoteOK rejects requests without a User-Agent.
	req.Header.Set("User-Agent", "JobPilot (local job search assistant)")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach RemoteOK: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remoteOK returned non-OK status: %d", resp.StatusCode)
	}

	var entries []remoteOKEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("failed to decode the RemoteOK feed: %w", err)
	}

	jobs := make([]domain.Job, 0, len(entries))
	for _, entry := range entries {
		if entry.Position == "" || entry.URL == "" {
			// The legal notice, or an entry too incomplete to act on.
			continue
		}
		jobs = append(jobs, s.toDomainJob(entry))
	}

	s.cache.put(jobs)
	return jobs, nil
}

// GetJob is unsupported: the feed has no by-URL endpoint.
func (s *RemoteOKSource) GetJob(context.Context, string) (*domain.Job, error) {
	return nil, fmt.Errorf("remoteOK cannot fetch a job by URL")
}

func (s *RemoteOKSource) toDomainJob(entry remoteOKEntry) domain.Job {
	description, err := htmlToMarkdown(entry.Description)
	if err != nil {
		description = entry.Description
	}

	var locations []string
	if entry.Location != "" {
		locations = []string{entry.Location}
	}

	postedAt, _ := time.Parse(time.RFC3339, entry.Date)
	raw, _ := json.Marshal(entry)

	job := domain.Job{
		CanonicalURL: entry.URL,
		SourceID:     s.ID(),
		SourceJobID:  entry.ID,
		// Trimmed: the feed carries trailing spaces in company names
		// often enough that "Bjak " would be a different employer from
		// "Bjak" everywhere the two are compared.
		Title:               strings.TrimSpace(entry.Position),
		CompanyName:         strings.TrimSpace(entry.Company),
		DescriptionMarkdown: description,
		Locations:           locations,
		// Every posting on RemoteOK is remote; that is the whole board.
		RemoteType:         "remote",
		Skills:             entry.Tags,
		PostedAt:           postedAt,
		DiscoveredAt:       time.Now(),
		RawSourceReference: string(raw),
	}

	// A zero salary means "not stated" rather than "unpaid", and most
	// entries state none. Reporting zero would make a job look like it
	// pays nothing and fail every salary filter.
	if entry.SalaryMin > 0 || entry.SalaryMax > 0 {
		job.SalaryMin = entry.SalaryMin
		job.SalaryMax = entry.SalaryMax
		job.SalaryCurrency = "USD"
		job.SalaryPeriod = "yearly"
	}

	return job
}
