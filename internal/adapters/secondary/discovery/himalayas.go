package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// DefaultHimalayasBaseURL is Himalayas' public, no-auth remote job API.
// See https://himalayas.app/jobs/api.
const DefaultHimalayasBaseURL = "https://himalayas.app/jobs/api"

// himalayasCacheTTL is an hour per query. Himalayas updates its listings
// daily, asks API users not to poll for nothing, and answers 429 to one
// that does.
const himalayasCacheTTL = time.Hour

// himalayasPageSize is the API's own cap. Asking for more returns 20
// anyway, so the number is written here rather than discovered at runtime.
const himalayasPageSize = 20

// himalayasMaxPages bounds one browse of the newest listings.
//
// The board holds around a hundred thousand jobs. Pulling all of them into
// a local database on every search is not breadth, it is a different
// product — and JobDiscoveryService filters client-side, so everything
// fetched is held in memory first. Four pages is a useful recent slice at
// a cost the user does not notice.
const himalayasMaxPages = 4

// himalayasMaxSearchPages bounds one search: the hundred most relevant
// results. Measured on 2026-09-30, "backend engineer" open to Nigeria had
// 219, and relevance thins out well before the last of them (ADR-0023 §6).
const himalayasMaxSearchPages = 5

// HimalayasSource is a ports.JobSource backed by Himalayas' public API.
//
// It is the richest of the no-key sources: unlike the others it reports a
// salary range with its currency and period, an employment type, and the
// countries a role is restricted to.
type HimalayasSource struct {
	baseURL    string
	httpClient *http.Client
	cache      *queryCache[domain.Job]
}

// NewHimalayasSource constructs a HimalayasSource targeting baseURL.
func NewHimalayasSource(baseURL string) *HimalayasSource {
	return &HimalayasSource{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 20 * time.Second},
		cache:      newQueryCache[domain.Job](himalayasCacheTTL),
	}
}

// ID returns the source's unique identifier.
func (s *HimalayasSource) ID() string { return "himalayas" }

// Name returns the source's human-readable name.
func (s *HimalayasSource) Name() string { return "Himalayas" }

// Capabilities reports search only.
func (s *HimalayasSource) Capabilities() ports.SourceCapabilities {
	return ports.SourceCapabilities{CanSearch: true, FiltersServerSide: false, CanGetJob: false}
}

type himalayasResponse struct {
	Jobs       []himalayasJob `json:"jobs"`
	TotalCount int            `json:"totalCount"`
}

type himalayasJob struct {
	GUID                 string   `json:"guid"`
	Title                string   `json:"title"`
	CompanyName          string   `json:"companyName"`
	CompanySlug          string   `json:"companySlug"`
	Description          string   `json:"description"`
	Excerpt              string   `json:"excerpt"`
	ApplicationLink      string   `json:"applicationLink"`
	LocationRestrictions []string `json:"locationRestrictions"`
	EmploymentType       string   `json:"employmentType"`
	MinSalary            float64  `json:"minSalary"`
	MaxSalary            float64  `json:"maxSalary"`
	Currency             string   `json:"currency"`
	SalaryPeriod         string   `json:"salaryPeriod"`
	Categories           []string `json:"categories"`
	PubDate              int64    `json:"pubDate"`
	ExpiryDate           int64    `json:"expiryDate"`
}

// Search asks Himalayas' search for the criteria's title and keywords, and
// for jobs open to the country the candidate works from; with neither
// title nor keywords, it browses the newest listings instead.
//
// Himalayas' search is its own relevance ranking rather than
// domain.SearchCriteria.Matches, so JobDiscoveryService still applies the
// criteria: the search decides which of the board's jobs are read, not
// which are kept (ADR-0023 §6).
func (s *HimalayasSource) Search(ctx context.Context, criteria domain.SearchCriteria) ([]domain.Job, error) {
	query := himalayasQuery(criteria)
	endpoint, pages := s.baseURL, himalayasMaxPages
	if query.Get("q") != "" {
		endpoint, pages = s.baseURL+"/search", himalayasMaxSearchPages
	}

	key := endpoint + "?" + query.Encode()
	if cached, fresh := s.cache.get(key); fresh {
		return cached, nil
	}

	var jobs []domain.Job
	seen := make(map[string]bool)
	for page := 0; page < pages; page++ {
		batch, err := s.fetchPage(ctx, endpoint, query, page)
		if err != nil {
			// A later page failing does not discard the earlier ones: a
			// partial slice of a hundred thousand jobs is still a useful
			// slice, and the alternative is no results at all.
			if len(jobs) > 0 {
				break
			}
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		for _, job := range batch {
			// Pages are read while the board changes underneath them, so
			// one posting can appear on two.
			if !seen[job.CanonicalURL] {
				seen[job.CanonicalURL] = true
				jobs = append(jobs, job)
			}
		}
	}

	s.cache.put(key, jobs)
	return jobs, nil
}

// himalayasQuery is what a search asks Himalayas for. The country is sent
// as its ISO code, which the search accepts, and it returns worldwide roles
// along with those that name the country.
func himalayasQuery(criteria domain.SearchCriteria) url.Values {
	query := url.Values{}
	if q := strings.Join(strings.Fields(criteria.Title+" "+criteria.Keywords), " "); q != "" {
		query.Set("q", q)
		if country := strings.TrimSpace(criteria.WorkFrom); country != "" {
			query.Set("country", strings.ToUpper(country))
		}
	}
	return query
}

// fetchPage reads one page. The browse endpoint pages by offset, the search
// endpoint by a page number counted from one.
func (s *HimalayasSource) fetchPage(ctx context.Context, endpoint string, base url.Values, page int) ([]domain.Job, error) {
	query := url.Values{}
	for key, values := range base {
		query[key] = values
	}
	if base.Get("q") != "" {
		query.Set("page", strconv.Itoa(page+1))
	} else {
		query.Set("limit", strconv.Itoa(himalayasPageSize))
		query.Set("offset", strconv.Itoa(page*himalayasPageSize))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "JobPilot (local job search assistant)")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach Himalayas: %w: %w", ports.ErrSourceUnreachable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("himalayas returned non-OK status: %d", resp.StatusCode)
	}

	var parsed himalayasResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode the Himalayas response: %w", err)
	}

	jobs := make([]domain.Job, 0, len(parsed.Jobs))
	for _, item := range parsed.Jobs {
		if item.Title == "" || item.ApplicationLink == "" {
			continue
		}
		jobs = append(jobs, s.toDomainJob(item))
	}
	return jobs, nil
}

// GetJob is unsupported.
func (s *HimalayasSource) GetJob(context.Context, string) (*domain.Job, error) {
	return nil, fmt.Errorf("himalayas cannot fetch a job by URL")
}

func (s *HimalayasSource) toDomainJob(item himalayasJob) domain.Job {
	description, err := htmlToMarkdown(item.Description)
	if err != nil {
		description = item.Description
	}
	if strings.TrimSpace(description) == "" {
		description = item.Excerpt
	}

	raw, _ := json.Marshal(item)

	job := domain.Job{
		CanonicalURL:        item.ApplicationLink,
		SourceID:            s.ID(),
		SourceJobID:         item.GUID,
		Title:               strings.TrimSpace(item.Title),
		CompanyName:         strings.TrimSpace(item.CompanyName),
		DescriptionMarkdown: description,
		Locations:           himalayasLocations(item.LocationRestrictions),
		RemoteType:          "remote",
		EmploymentType:      normalizeEmploymentType(item.EmploymentType),
		Skills:              item.Categories,
		DiscoveredAt:        time.Now(),
		RawSourceReference:  string(raw),
	}

	if item.PubDate > 0 {
		job.PostedAt = time.Unix(item.PubDate, 0).UTC()
	}
	if item.ExpiryDate > 0 {
		job.ExpiresAt = time.Unix(item.ExpiryDate, 0).UTC()
	}
	if item.MinSalary > 0 || item.MaxSalary > 0 {
		job.SalaryMin = item.MinSalary
		job.SalaryMax = item.MaxSalary
		job.SalaryCurrency = strings.ToUpper(item.Currency)
		job.SalaryPeriod = strings.ToLower(item.SalaryPeriod)
	}
	return job
}

// himalayasLocations is where a Himalayas job can be done from, in words.
// Himalayas encodes "worldwide" as an empty restriction list, which its own
// worldwide search confirms, and an absence would read as "not stated"
// (ADR-0023 §1), so it is written out.
func himalayasLocations(restrictions []string) []string {
	if len(restrictions) == 0 {
		return []string{"Worldwide"}
	}
	return restrictions
}

// normalizeEmploymentType maps a source's spelling onto the vocabulary the
// rest of the product compares against. An unrecognized value is dropped
// rather than stored: a value nothing can compare silently fails every
// filter instead of being visibly absent.
func normalizeEmploymentType(value string) string {
	simplify := strings.NewReplacer(" ", "", "_", "", "-", "")
	simplified := simplify.Replace(strings.ToLower(strings.TrimSpace(value)))
	for _, known := range []string{"full-time", "part-time", "contract", "internship", "temporary"} {
		if simplify.Replace(known) == simplified {
			return known
		}
	}
	switch simplified {
	case "fulltimeemployee", "permanent":
		return "full-time"
	case "intern":
		return "internship"
	case "freelance", "contractor":
		return "contract"
	}
	return ""
}
