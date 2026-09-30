package discovery

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// DefaultWeWorkRemotelyBaseURL is where We Work Remotely publishes its
// category feeds. See https://weworkremotely.com/remote-job-rss-feed.
const DefaultWeWorkRemotelyBaseURL = "https://weworkremotely.com/categories"

// weWorkRemotelyCategories are the engineering feeds read. Measured on
// 2026-09-30 they held 68 distinct postings between them, most in more than
// one. The board's other categories — sales, design, support — are not a
// software engineer's, and core would filter them out anyway.
var weWorkRemotelyCategories = []string{
	"remote-programming-jobs",
	"remote-full-stack-programming-jobs",
	"remote-back-end-programming-jobs",
	"remote-front-end-programming-jobs",
	"remote-devops-sysadmin-jobs",
}

// weWorkRemotelyCacheTTL matches the feeds' own <ttl> of 60 minutes.
const weWorkRemotelyCacheTTL = time.Hour

// WeWorkRemotelySource is a ports.JobSource backed by We Work Remotely's
// public RSS feeds.
//
// Its terms ask only for attribution: "Anyone can use the feed, all we ask
// is that you attribute the links back to We Work Remotely." Every job keeps
// its We Work Remotely URL as its canonical URL (ADR-0023 §7).
type WeWorkRemotelySource struct {
	baseURL    string
	httpClient *http.Client
	cache      *jobCache[domain.Job]
}

// NewWeWorkRemotelySource constructs a WeWorkRemotelySource reading the
// category feeds under baseURL.
func NewWeWorkRemotelySource(baseURL string) *WeWorkRemotelySource {
	return &WeWorkRemotelySource{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{Timeout: 20 * time.Second},
		cache:      newJobCache[domain.Job](weWorkRemotelyCacheTTL),
	}
}

// ID returns the source's unique identifier.
func (s *WeWorkRemotelySource) ID() string { return "weworkremotely" }

// Name returns the source's human-readable name.
func (s *WeWorkRemotelySource) Name() string { return "We Work Remotely" }

// Capabilities reports search only; the feeds take no query.
func (s *WeWorkRemotelySource) Capabilities() ports.SourceCapabilities {
	return ports.SourceCapabilities{CanSearch: true, FiltersServerSide: false, CanGetJob: false}
}

type weWorkRemotelyFeed struct {
	Items []weWorkRemotelyItem `xml:"channel>item"`
}

// weWorkRemotelyItem is one posting. Region is the board's label for where
// a job can be done from; Country, when present, is the list of countries
// the employer chose, which is the narrower and the truer of the two.
type weWorkRemotelyItem struct {
	Title       string `xml:"title"`
	Region      string `xml:"region"`
	Country     string `xml:"country"`
	Skills      string `xml:"skills"`
	Category    string `xml:"category"`
	Type        string `xml:"type"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	ExpiresAt   string `xml:"expires_at"`
	GUID        string `xml:"guid"`
	Link        string `xml:"link"`
}

// Search returns every posting in the engineering feeds, each once;
// JobDiscoveryService applies the criteria.
//
// The feeds are read at once. A feed that fails leaves the others' postings
// in the result, since each feed is a slice of the same board; the search
// fails only when none answered.
func (s *WeWorkRemotelySource) Search(ctx context.Context, _ domain.SearchCriteria) ([]domain.Job, error) {
	if cached, fresh := s.cache.get(); fresh {
		return cached, nil
	}

	feeds := make([][]weWorkRemotelyItem, len(weWorkRemotelyCategories))
	errs := make([]error, len(weWorkRemotelyCategories))
	var wg sync.WaitGroup
	for i, category := range weWorkRemotelyCategories {
		wg.Go(func() {
			feeds[i], errs[i] = s.fetchFeed(ctx, category)
		})
	}
	wg.Wait()

	if err := noFeedAnswered(errs); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var jobs []domain.Job
	for _, items := range feeds {
		for _, item := range items {
			if item.Link == "" || item.Title == "" || seen[item.Link] {
				continue
			}
			seen[item.Link] = true
			jobs = append(jobs, s.toDomainJob(item))
		}
	}
	s.cache.put(jobs)
	return jobs, nil
}

// noFeedAnswered is the error for a search no feed answered, or nil when
// one did. It is unreachable only when every feed was: one that was reached
// and failed means the connection is not the reason.
func noFeedAnswered(errs []error) error {
	var failed error
	for _, err := range errs {
		switch {
		case err == nil:
			return nil
		case !errors.Is(err, ports.ErrSourceUnreachable) && failed == nil:
			failed = err
		}
	}
	if failed != nil {
		return failed
	}
	return errs[0]
}

func (s *WeWorkRemotelySource) fetchFeed(ctx context.Context, category string) ([]weWorkRemotelyItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/"+category+".rss", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "JobPilot (local job search assistant)")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach We Work Remotely: %w: %w", ports.ErrSourceUnreachable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the We Work Remotely %s feed returned non-OK status: %d", category, resp.StatusCode)
	}

	var feed weWorkRemotelyFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("failed to decode We Work Remotely's %s feed: %w", category, err)
	}
	return feed.Items, nil
}

// GetJob is unsupported.
func (s *WeWorkRemotelySource) GetJob(context.Context, string) (*domain.Job, error) {
	return nil, fmt.Errorf("a job cannot be fetched from We Work Remotely by URL")
}

// skillSeparators split "Java, PostgreSQL, and Spring".
var skillSeparators = regexp.MustCompile(`\s*,\s*(?:and\s+)?|\s+and\s+`)

func (s *WeWorkRemotelySource) toDomainJob(item weWorkRemotelyItem) domain.Job {
	// A title is "Company: Role"; a role can hold a colon of its own.
	company, title, found := strings.Cut(item.Title, ": ")
	if !found {
		company, title = "", item.Title
	}

	description, err := htmlToMarkdown(item.Description)
	if err != nil {
		description = item.Description
	}

	// The country list is the employer's own restriction, where the region
	// is the board's label: measured, 21 of 63 postings labelled "Anywhere
	// in the World" named countries, most of them the United States alone,
	// and their descriptions agreed with the countries (ADR-0023 §7).
	location := strings.TrimSpace(item.Country)
	if location == "" {
		location = strings.TrimSpace(item.Region)
	}
	var locations []string
	if location != "" {
		locations = []string{location}
	}

	var skills []string
	for _, skill := range skillSeparators.Split(strings.TrimSpace(item.Skills), -1) {
		if skill = strings.TrimSpace(skill); skill != "" {
			skills = append(skills, skill)
		}
	}

	job := domain.Job{
		CanonicalURL:        item.Link,
		SourceID:            s.ID(),
		SourceJobID:         item.GUID,
		Title:               strings.TrimSpace(title),
		CompanyName:         strings.TrimSpace(company),
		DescriptionMarkdown: description,
		Locations:           locations,
		// Every posting on We Work Remotely is remote; that is the board.
		RemoteType:         domain.RemoteTypeRemote,
		EmploymentType:     normalizeEmploymentType(item.Type),
		Skills:             skills,
		DiscoveredAt:       time.Now(),
		RawSourceReference: item.Link,
	}
	if posted, err := time.Parse(time.RFC1123Z, item.PubDate); err == nil {
		job.PostedAt = posted.UTC()
	}
	if expires, err := time.Parse(time.RFC1123Z, item.ExpiresAt); err == nil {
		job.ExpiresAt = expires.UTC()
	}
	return job
}
