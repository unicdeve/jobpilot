package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/domain/geo"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// DefaultJobicyBaseURL is Jobicy's public remote-jobs API. See
// https://jobicy.com/jobs-rss-feed.
const DefaultJobicyBaseURL = "https://jobicy.com/api/v2/remote-jobs"

// jobicyCacheTTL is the least Jobicy allows: "Automated checks must not run
// more frequently than once per hour." Every feed is cached for it, whatever
// asks — Discover, a Scout, a launch refresh — so no feed is asked twice in
// an hour (ADR-0023 §7).
const jobicyCacheTTL = time.Hour

// jobicyLocationsTTL is how long Jobicy's list of locations is kept. Jobicy
// asks for it to be read rather than hard-coded, and it changes rarely.
const jobicyLocationsTTL = 24 * time.Hour

// jobicyCount is the most one request returns.
const jobicyCount = 200

// jobicyAnywhere is Jobicy's slug for a job open anywhere.
const jobicyAnywhere = "anywhere"

// JobicySource is a ports.JobSource backed by Jobicy's public API.
//
// It is asked for the feeds a candidate could take a job from: jobs open
// anywhere, and each of Jobicy's own locations that holds the country the
// search works from — for Nigeria, EMEA. Without a country it reads the
// newest jobs from everywhere. It is never asked for a keyword: every saved
// search then shares the same few feeds, and core applies the criteria.
//
// Jobicy's terms ask that it stay credited as the source with its own URL,
// so every job keeps its Jobicy URL as its canonical URL.
type JobicySource struct {
	baseURL    string
	httpClient *http.Client
	cache      *queryCache[domain.Job]

	mu          sync.Mutex
	locations   []jobicyLocation
	locationsAt time.Time
}

// NewJobicySource constructs a JobicySource targeting baseURL.
func NewJobicySource(baseURL string) *JobicySource {
	return &JobicySource{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 20 * time.Second},
		cache:      newQueryCache[domain.Job](jobicyCacheTTL),
	}
}

// ID returns the source's unique identifier.
func (s *JobicySource) ID() string { return "jobicy" }

// Name returns the source's human-readable name.
func (s *JobicySource) Name() string { return "Jobicy" }

// Capabilities reports search only.
func (s *JobicySource) Capabilities() ports.SourceCapabilities {
	return ports.SourceCapabilities{CanSearch: true, FiltersServerSide: false, CanGetJob: false}
}

type jobicyLocation struct {
	Name string `json:"geoName"`
	Slug string `json:"geoSlug"`
}

type jobicyResponse struct {
	Jobs      []jobicyJob      `json:"jobs"`
	Locations []jobicyLocation `json:"locations"`
}

type jobicyJob struct {
	ID             int64    `json:"id"`
	URL            string   `json:"url"`
	Title          string   `json:"jobTitle"`
	CompanyName    string   `json:"companyName"`
	Industry       []string `json:"jobIndustry"`
	Type           []string `json:"jobType"`
	Geo            string   `json:"jobGeo"`
	Description    string   `json:"jobDescription"`
	Excerpt        string   `json:"jobExcerpt"`
	PubDate        string   `json:"pubDate"`
	SalaryMin      float64  `json:"salaryMin"`
	SalaryMax      float64  `json:"salaryMax"`
	SalaryCurrency string   `json:"salaryCurrency"`
	SalaryPeriod   string   `json:"salaryPeriod"`
}

// Search reads the feeds for the criteria's country, each once, and joins
// them. JobDiscoveryService applies the criteria.
func (s *JobicySource) Search(ctx context.Context, criteria domain.SearchCriteria) ([]domain.Job, error) {
	feeds, err := s.feedsFor(ctx, criteria.WorkFrom)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var jobs []domain.Job
	for _, geoSlug := range feeds {
		batch, err := s.fetchFeed(ctx, geoSlug)
		if err != nil {
			// A feed after the first failing keeps what the others found.
			if len(jobs) > 0 {
				break
			}
			return nil, err
		}
		for _, job := range batch {
			if !seen[job.CanonicalURL] {
				seen[job.CanonicalURL] = true
				jobs = append(jobs, job)
			}
		}
	}
	return jobs, nil
}

// feedsFor is the geo slugs to read for a candidate working from country:
// none, meaning every job, when there is no country; otherwise "anywhere"
// and each Jobicy location that contains the country.
func (s *JobicySource) feedsFor(ctx context.Context, country string) ([]string, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if country == "" {
		return []string{""}, nil
	}
	locations, err := s.readLocations(ctx)
	if err != nil {
		return nil, err
	}
	feeds := []string{jobicyAnywhere}
	for _, location := range locations {
		if location.Slug == jobicyAnywhere {
			continue
		}
		place, ok := geo.Lookup(location.Name, false)
		if ok && slices.Contains(place.Countries, country) {
			feeds = append(feeds, location.Slug)
		}
	}
	return feeds, nil
}

// readLocations returns Jobicy's locations, reading them at most daily.
func (s *JobicySource) readLocations(ctx context.Context) ([]jobicyLocation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locations != nil && time.Since(s.locationsAt) < jobicyLocationsTTL {
		return s.locations, nil
	}
	parsed, err := s.get(ctx, url.Values{"get": {"locations"}})
	if err != nil {
		return nil, err
	}
	s.locations, s.locationsAt = parsed.Locations, time.Now()
	return s.locations, nil
}

func (s *JobicySource) fetchFeed(ctx context.Context, geoSlug string) ([]domain.Job, error) {
	query := url.Values{"count": {fmt.Sprint(jobicyCount)}}
	if geoSlug != "" {
		query.Set("geo", geoSlug)
	}
	key := query.Encode()
	if cached, fresh := s.cache.get(key); fresh {
		return cached, nil
	}

	parsed, err := s.get(ctx, query)
	if err != nil {
		return nil, err
	}
	jobs := make([]domain.Job, 0, len(parsed.Jobs))
	for _, item := range parsed.Jobs {
		if item.Title == "" || item.URL == "" {
			continue
		}
		jobs = append(jobs, s.toDomainJob(item))
	}
	s.cache.put(key, jobs)
	return jobs, nil
}

func (s *JobicySource) get(ctx context.Context, query url.Values) (*jobicyResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "JobPilot (local job search assistant)")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach Jobicy: %w: %w", ports.ErrSourceUnreachable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jobicy returned non-OK status: %d", resp.StatusCode)
	}
	var parsed jobicyResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode the Jobicy response: %w", err)
	}
	return &parsed, nil
}

// GetJob is unsupported.
func (s *JobicySource) GetJob(context.Context, string) (*domain.Job, error) {
	return nil, fmt.Errorf("jobicy cannot fetch a job by URL")
}

func (s *JobicySource) toDomainJob(item jobicyJob) domain.Job {
	description, err := htmlToMarkdown(item.Description)
	if err != nil {
		description = item.Description
	}
	if strings.TrimSpace(description) == "" {
		description = item.Excerpt
	}

	// jobGeo is Jobicy's own list, "Canada,  USA", or "Anywhere".
	var locations []string
	for _, place := range strings.Split(item.Geo, ",") {
		if place = strings.TrimSpace(place); place != "" {
			locations = append(locations, place)
		}
	}

	employment := ""
	if len(item.Type) > 0 {
		employment = normalizeEmploymentType(item.Type[0])
	}

	raw, _ := json.Marshal(item)
	job := domain.Job{
		CanonicalURL:        item.URL,
		SourceID:            s.ID(),
		SourceJobID:         fmt.Sprint(item.ID),
		Title:               strings.TrimSpace(item.Title),
		CompanyName:         strings.TrimSpace(item.CompanyName),
		DescriptionMarkdown: description,
		Locations:           locations,
		RemoteType:          domain.RemoteTypeRemote,
		EmploymentType:      employment,
		Skills:              item.Industry,
		DiscoveredAt:        time.Now(),
		RawSourceReference:  string(raw),
	}
	if posted, err := time.Parse(time.RFC3339, item.PubDate); err == nil {
		job.PostedAt = posted.UTC()
	}
	if item.SalaryMin > 0 || item.SalaryMax > 0 {
		job.SalaryMin = item.SalaryMin
		job.SalaryMax = item.SalaryMax
		job.SalaryCurrency = strings.ToUpper(item.SalaryCurrency)
		job.SalaryPeriod = strings.ToLower(item.SalaryPeriod)
	}
	return job
}
