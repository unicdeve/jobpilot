package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// companyBoardTTL is how long a board's answer is kept (spec D5): running
// every saved search asks each company once.
const companyBoardTTL = time.Hour

// maxBoardBody bounds one board's answer. OpenAI's Ashby board, the largest
// measured on 2026-09-30, is 834 postings in about 15 MB.
const maxBoardBody = 64 << 20

// BoardEndpoints are where CompanyBoards reads each provider: the public
// APIs, and the hosted pages that name a Lever or Ashby company or say
// whether a SmartRecruiters company exists.
type BoardEndpoints struct {
	GreenhouseAPI        string
	GreenhouseEUAPI      string
	LeverAPI             string
	LeverEUAPI           string
	LeverPages           string
	LeverEUPages         string
	AshbyAPI             string
	AshbyPages           string
	WorkableAPI          string
	SmartRecruitersAPI   string
	SmartRecruitersPages string
}

// DefaultBoardEndpoints are the providers' own. See
// docs/02_features/17_company_watchlists.md for what each documents.
var DefaultBoardEndpoints = BoardEndpoints{
	GreenhouseAPI:        "https://boards-api.greenhouse.io",
	GreenhouseEUAPI:      "https://boards-api.eu.greenhouse.io",
	LeverAPI:             "https://api.lever.co",
	LeverEUAPI:           "https://api.eu.lever.co",
	LeverPages:           "https://jobs.lever.co",
	LeverEUPages:         "https://jobs.eu.lever.co",
	AshbyAPI:             "https://api.ashbyhq.com",
	AshbyPages:           "https://jobs.ashbyhq.com",
	WorkableAPI:          "https://apply.workable.com",
	SmartRecruitersAPI:   "https://api.smartrecruiters.com",
	SmartRecruitersPages: "https://careers.smartrecruiters.com",
}

// companyPagesSourceID is the source a job read from a company's own page
// is stored under. It is the ID of the source that asks for it, in core.
const companyPagesSourceID = "company-pages"

// CompanyBoards is a ports.BoardReader for Greenhouse, Lever, Ashby,
// Workable and SmartRecruiters (ADR-0024 §4, ADR-0026). It knows boards,
// not who watches them.
type CompanyBoards struct {
	endpoints  BoardEndpoints
	httpClient *http.Client
	// oneHop is httpClient without following redirects, for the answers
	// whose redirect is what they say.
	oneHop *http.Client

	flight singleflight.Group
	mu     sync.Mutex
	cache  map[string]*domain.BoardContents
	// names are the companies' names as the last read of a Workable or
	// SmartRecruiters board gave them, which is where those boards name
	// their company.
	names map[string]string

	descriptions descriptions
}

// NewCompanyBoards constructs a CompanyBoards reading endpoints.
func NewCompanyBoards(endpoints BoardEndpoints) *CompanyBoards {
	return &CompanyBoards{
		endpoints:  endpoints,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		oneHop: &http.Client{
			Timeout:       30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		cache:        make(map[string]*domain.BoardContents),
		names:        make(map[string]string),
		descriptions: descriptions{byBoard: make(map[string]map[string]describedPosting)},
	}
}

func boardKey(board domain.Board) string {
	return string(board.Provider) + "/" + board.Region + "/" + strings.ToLower(board.Name)
}

// ReadBoard returns the board's open postings, from the last hour's answer
// when there is one. Reads of one board at once share one request.
func (b *CompanyBoards) ReadBoard(ctx context.Context, board domain.Board) (*domain.BoardContents, error) {
	key := boardKey(board)
	b.mu.Lock()
	cached, ok := b.cache[key]
	b.mu.Unlock()
	if ok && time.Since(cached.FetchedAt) < companyBoardTTL {
		return cached, nil
	}

	// The request outlives the caller that started it, since others may be
	// waiting on it; each caller still stops waiting when it is cancelled.
	shared := context.WithoutCancel(ctx)
	answer := b.flight.DoChan(key, func() (any, error) {
		jobs, name, err := b.fetch(shared, board)
		if err != nil {
			return nil, err
		}
		contents := &domain.BoardContents{
			Jobs:        jobs,
			FetchedAt:   time.Now().UTC(),
			Undescribed: board.Provider == domain.ProviderSmartRecruiters,
		}
		if contents.Undescribed {
			b.descriptions.keepOnly(key, jobs)
		}
		b.mu.Lock()
		b.cache[key] = contents
		b.names[key] = name
		b.mu.Unlock()
		return contents, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-answer:
		if result.Err != nil {
			return nil, result.Err
		}
		return result.Val.(*domain.BoardContents), nil
	}
}

// fetch reads a board's postings, and the company's name where the same
// answer gives it.
func (b *CompanyBoards) fetch(ctx context.Context, board domain.Board) ([]domain.Job, string, error) {
	escaped := url.PathEscape(board.Name)
	switch board.Provider {
	case domain.ProviderGreenhouse:
		var answer greenhouseBoard
		if err := b.getJSON(ctx, b.greenhouseAPI(board)+"/v1/boards/"+escaped+"/jobs?content=true", &answer); err != nil {
			return nil, "", err
		}
		return mapJobs(answer.Jobs, func(job greenhouseJob) (domain.Job, bool) { return greenhouseToJob(board, job) }), "", nil
	case domain.ProviderLever:
		api := b.endpoints.LeverAPI
		if board.Region == domain.RegionEU {
			api = b.endpoints.LeverEUAPI
		}
		var answer []leverJob
		if err := b.getJSON(ctx, api+"/v0/postings/"+escaped+"?mode=json", &answer); err != nil {
			return nil, "", err
		}
		return mapJobs(answer, func(job leverJob) (domain.Job, bool) { return leverToJob(board, job) }), "", nil
	case domain.ProviderAshby:
		var answer ashbyBoard
		if err := b.getJSON(ctx, b.endpoints.AshbyAPI+"/posting-api/job-board/"+escaped+"?includeCompensation=true", &answer); err != nil {
			return nil, "", err
		}
		return mapJobs(answer.Jobs, func(job ashbyJob) (domain.Job, bool) { return ashbyToJob(board, job) }), "", nil
	case domain.ProviderWorkable:
		return b.readWorkable(ctx, board)
	case domain.ProviderSmartRecruiters:
		return b.readSmartRecruiters(ctx, board)
	}
	return nil, "", fmt.Errorf("unknown provider %q", board.Provider)
}

func (b *CompanyBoards) greenhouseAPI(board domain.Board) string {
	if board.Region == domain.RegionEU {
		return b.endpoints.GreenhouseEUAPI
	}
	return b.endpoints.GreenhouseAPI
}

func mapJobs[T any](items []T, toJob func(T) (domain.Job, bool)) []domain.Job {
	jobs := make([]domain.Job, 0, len(items))
	for _, item := range items {
		if job, ok := toJob(item); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs
}

// BoardName returns the company's name as its board gives it: Greenhouse's
// board endpoint, the hosted page Lever and Ashby name the company on, or
// the board's own answer for Workable and SmartRecruiters, which is read
// once for both. It is "" when none gives one.
func (b *CompanyBoards) BoardName(ctx context.Context, board domain.Board) (string, error) {
	escaped := url.PathEscape(board.Name)
	switch board.Provider {
	case domain.ProviderWorkable, domain.ProviderSmartRecruiters:
		// Workable limits how often it is asked, so the name comes from the
		// read a watch makes anyway (ADR-0026 §5).
		if _, err := b.ReadBoard(ctx, board); err != nil {
			return "", err
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		return b.names[boardKey(board)], nil
	case domain.ProviderGreenhouse:
		var answer struct {
			Name string `json:"name"`
		}
		if err := b.getJSON(ctx, b.greenhouseAPI(board)+"/v1/boards/"+escaped, &answer); err != nil {
			return "", err
		}
		return domain.CleanCompanyName(answer.Name), nil
	case domain.ProviderLever:
		pages := b.endpoints.LeverPages
		if board.Region == domain.RegionEU {
			pages = b.endpoints.LeverEUPages
		}
		// Lever's API answers a site that does not exist with an empty
		// list; its page answers 404.
		page, err := b.get(ctx, pages+"/"+escaped)
		if err != nil {
			return "", err
		}
		return leverPageName(page), nil
	case domain.ProviderAshby:
		page, err := b.get(ctx, b.endpoints.AshbyPages+"/"+escaped)
		if err != nil {
			return "", err
		}
		return ashbyPageName(page), nil
	}
	return "", fmt.Errorf("unknown provider %q", board.Provider)
}

var (
	pageTitle = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	// ashbyOrganization is the organization Ashby's page embeds for its
	// script: "organization":{"organizationId":"…","name":"ClickUp",…}.
	ashbyOrganization = regexp.MustCompile(`"organization":\{[^{}]*?"name":("(?:[^"\\]|\\.)*")`)
)

// leverPageName is the company a Lever page is titled with: "Spotify".
func leverPageName(page []byte) string {
	match := pageTitle.FindSubmatch(page)
	if match == nil {
		return ""
	}
	return domain.CleanCompanyName(html.UnescapeString(string(match[1])))
}

// ashbyPageName is the name of the organization an Ashby page embeds.
func ashbyPageName(page []byte) string {
	match := ashbyOrganization.FindSubmatch(page)
	if match == nil {
		return ""
	}
	var name string
	if err := json.Unmarshal(match[1], &name); err != nil {
		return ""
	}
	return domain.CleanCompanyName(name)
}

func (b *CompanyBoards) getJSON(ctx context.Context, address string, into any) error {
	body, err := b.get(ctx, address)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("failed to read the board's answer: %w", err)
	}
	return nil
}

// get reads address, and says which way it failed: ports.ErrBoardGone for
// a 404, ports.ErrSourceUnreachable for no answer.
func (b *CompanyBoards) get(ctx context.Context, address string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "JobPilot (local job search assistant)")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach the hiring page: %w: %w", ports.ErrSourceUnreachable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, ports.ErrBoardGone
	case resp.StatusCode == http.StatusTooManyRequests:
		// Workable's answer when asked too often (ADR-0026 §5).
		return nil, ports.ErrBoardBusy
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("the hiring page answered with status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBoardBody+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read the hiring page: %w", err)
	}
	if len(body) > maxBoardBody {
		return nil, errors.New("the hiring page's answer is larger than JobPilot reads")
	}
	return body, nil
}

// getOnce reads address without following a redirect, and returns the
// page, or where a redirect leads. A 404 is ports.ErrBoardGone.
func (b *CompanyBoards) getOnce(ctx context.Context, address string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "JobPilot (local job search assistant)")

	resp, err := b.oneHop.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to reach the hiring page: %w: %w", ports.ErrSourceUnreachable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		location, err := resp.Location()
		if err != nil {
			return nil, "", fmt.Errorf("the hiring page redirected without saying where: %w", err)
		}
		return nil, location.String(), nil
	case resp.StatusCode == http.StatusNotFound:
		return nil, "", ports.ErrBoardGone
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, "", ports.ErrBoardBusy
	case resp.StatusCode != http.StatusOK:
		return nil, "", fmt.Errorf("the hiring page answered with status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBoardBody+1))
	if err != nil {
		return nil, "", fmt.Errorf("failed to read the hiring page: %w", err)
	}
	if len(body) > maxBoardBody {
		return nil, "", errors.New("the hiring page's answer is larger than JobPilot reads")
	}
	return body, "", nil
}

// ResolveAddress follows one redirect from address: a Workable posting's
// short address leads to the one naming its account. A posting Workable
// does not have is ports.ErrBoardGone.
func (b *CompanyBoards) ResolveAddress(ctx context.Context, address string) (string, error) {
	if !strings.Contains(address, "://") {
		address = "https://" + strings.TrimSpace(address)
	}
	_, location, err := b.getOnce(ctx, address)
	if err != nil {
		return "", err
	}
	if location == "" {
		return "", errors.New("the address did not lead to a company's page")
	}
	base, err := url.Parse(address)
	if err != nil {
		return "", fmt.Errorf("failed to read the address: %w", err)
	}
	target, err := base.Parse(location)
	if err != nil {
		return "", fmt.Errorf("failed to read where the address leads: %w", err)
	}
	return target.String(), nil
}

// --- Greenhouse ---

type greenhouseBoard struct {
	Jobs []greenhouseJob `json:"jobs"`
}

type greenhouseJob struct {
	ID          int64  `json:"id"`
	AbsoluteURL string `json:"absolute_url"`
	Title       string `json:"title"`
	CompanyName string `json:"company_name"`
	// Content is HTML, escaped once more as text.
	Content  string `json:"content"`
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
	FirstPublished string `json:"first_published"`
	UpdatedAt      string `json:"updated_at"`
}

var (
	remoteWord = regexp.MustCompile(`(?i)\bremote\b`)
	hybridWord = regexp.MustCompile(`(?i)\bhybrid\b`)
	onsiteWord = regexp.MustCompile(`(?i)\b(on-?site|in[- ]office)\b`)
)

func greenhouseToJob(board domain.Board, item greenhouseJob) (domain.Job, bool) {
	if item.Title == "" || item.AbsoluteURL == "" {
		return domain.Job{}, false
	}
	// A posting's locations, which Greenhouse writes as one string:
	// "Remote, Canada; Remote, United Kingdom; Remote, United States".
	locations := splitLocations(item.Location.Name, ";")

	job := boardJob(board, strconv.FormatInt(item.ID, 10), item.AbsoluteURL, item.Title,
		describe(html.UnescapeString(item.Content)), locations)
	job.CompanyName = strings.TrimSpace(item.CompanyName)
	// Greenhouse has no workplace field; its locations say it in words.
	job.RemoteType = remoteTypeInWords(item.Location.Name)
	job.PostedAt = parseTime(item.FirstPublished, item.UpdatedAt)
	return job, true
}

// remoteTypeInWords reads a remote type from a location's words: remote if
// it can be done remotely at all, and otherwise hybrid or onsite if it says
// so. A location that says none of them leaves the type unknown.
func remoteTypeInWords(location string) string {
	switch {
	case remoteWord.MatchString(location):
		return domain.RemoteTypeRemote
	case hybridWord.MatchString(location):
		return domain.RemoteTypeHybrid
	case onsiteWord.MatchString(location):
		return domain.RemoteTypeOnsite
	}
	return ""
}

// --- Lever ---

type leverJob struct {
	ID          string `json:"id"`
	Text        string `json:"text"`
	HostedURL   string `json:"hostedUrl"`
	Description string `json:"description"`
	Lists       []struct {
		Text    string `json:"text"`
		Content string `json:"content"`
	} `json:"lists"`
	Additional string `json:"additional"`
	Categories struct {
		Commitment   string   `json:"commitment"`
		Location     string   `json:"location"`
		AllLocations []string `json:"allLocations"`
	} `json:"categories"`
	WorkplaceType string `json:"workplaceType"`
	CreatedAt     int64  `json:"createdAt"`
	SalaryRange   *struct {
		Currency string  `json:"currency"`
		Interval string  `json:"interval"`
		Min      float64 `json:"min"`
		Max      float64 `json:"max"`
	} `json:"salaryRange"`
}

func leverToJob(board domain.Board, item leverJob) (domain.Job, bool) {
	if item.Text == "" || item.HostedURL == "" {
		return domain.Job{}, false
	}
	// A Lever posting's description is its opening, its lists and a
	// closing section, each HTML.
	var page strings.Builder
	page.WriteString(item.Description)
	for _, list := range item.Lists {
		fmt.Fprintf(&page, "<h3>%s</h3><ul>%s</ul>", html.EscapeString(list.Text), list.Content)
	}
	page.WriteString(item.Additional)

	locations := item.Categories.AllLocations
	if len(locations) == 0 {
		locations = splitLocations(item.Categories.Location, ";")
	}
	job := boardJob(board, item.ID, item.HostedURL, item.Text, describe(page.String()), trimAll(locations))
	job.RemoteType = workplaceType(item.WorkplaceType)
	job.EmploymentType = normalizeEmploymentType(item.Categories.Commitment)
	if item.CreatedAt > 0 {
		job.PostedAt = time.UnixMilli(item.CreatedAt).UTC()
	}
	if salary := item.SalaryRange; salary != nil && (salary.Min > 0 || salary.Max > 0) {
		job.SalaryMin, job.SalaryMax = salary.Min, salary.Max
		job.SalaryCurrency = strings.ToUpper(salary.Currency)
		job.SalaryPeriod = leverSalaryPeriod(salary.Interval)
	}
	return job, true
}

// leverSalaryPeriod reads Lever's interval: "per-year-salary",
// "per-hour-wage".
func leverSalaryPeriod(interval string) string {
	for _, period := range []string{"year", "month", "week", "day", "hour"} {
		if strings.Contains(interval, period) {
			return period
		}
	}
	return ""
}

// --- Ashby ---

type ashbyBoard struct {
	Jobs []ashbyJob `json:"jobs"`
}

type ashbyJob struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	JobURL             string `json:"jobUrl"`
	DescriptionHTML    string `json:"descriptionHtml"`
	EmploymentType     string `json:"employmentType"`
	Location           string `json:"location"`
	SecondaryLocations []struct {
		Location string `json:"location"`
	} `json:"secondaryLocations"`
	WorkplaceType string `json:"workplaceType"`
	IsRemote      bool   `json:"isRemote"`
	// IsListed is false for a posting reachable by its link but not shown
	// on the board, which the company has not published.
	IsListed     *bool  `json:"isListed"`
	PublishedAt  string `json:"publishedAt"`
	Compensation *struct {
		SummaryComponents []struct {
			CompensationType string   `json:"compensationType"`
			Interval         string   `json:"interval"`
			CurrencyCode     string   `json:"currencyCode"`
			MinValue         *float64 `json:"minValue"`
			MaxValue         *float64 `json:"maxValue"`
		} `json:"summaryComponents"`
	} `json:"compensation"`
}

func ashbyToJob(board domain.Board, item ashbyJob) (domain.Job, bool) {
	if item.Title == "" || item.JobURL == "" || (item.IsListed != nil && !*item.IsListed) {
		return domain.Job{}, false
	}
	// The posting's address is not a location: it names the office or
	// hiring entity, and a role open anywhere carries one (ADR-0024 §6).
	locations := []string{item.Location}
	for _, secondary := range item.SecondaryLocations {
		locations = append(locations, secondary.Location)
	}
	job := boardJob(board, item.ID, item.JobURL, item.Title, describe(item.DescriptionHTML), trimAll(locations))
	job.RemoteType = workplaceType(item.WorkplaceType)
	if job.RemoteType == "" && item.IsRemote {
		job.RemoteType = domain.RemoteTypeRemote
	}
	job.EmploymentType = normalizeEmploymentType(item.EmploymentType)
	job.PostedAt = parseTime(item.PublishedAt)
	if item.Compensation != nil {
		for _, component := range item.Compensation.SummaryComponents {
			if component.CompensationType != "Salary" || (component.MinValue == nil && component.MaxValue == nil) {
				continue
			}
			if component.MinValue != nil {
				job.SalaryMin = *component.MinValue
			}
			if component.MaxValue != nil {
				job.SalaryMax = *component.MaxValue
			}
			job.SalaryCurrency = strings.ToUpper(component.CurrencyCode)
			// "1 YEAR", "1 HOUR".
			job.SalaryPeriod = leverSalaryPeriod(strings.ToLower(component.Interval))
			break
		}
	}
	return job, true
}

// --- shared ---

// boardJob is what every provider's posting has in common. The company's
// name is left to its board, or to core, which knows what the candidate
// calls it.
func boardJob(board domain.Board, id, address, title, description string, locations []string) domain.Job {
	return domain.Job{
		CanonicalURL:        address,
		SourceID:            companyPagesSourceID,
		SourceJobID:         string(board.Provider) + "/" + board.Name + "/" + id,
		Title:               strings.TrimSpace(title),
		CompanyURL:          board.URL(),
		DescriptionMarkdown: description,
		Locations:           locations,
		DiscoveredAt:        time.Now(),
	}
}

// describe converts a posting's HTML to the markdown stored on the job.
func describe(page string) string {
	markdown, err := htmlToMarkdown(page)
	if err != nil {
		return page
	}
	return markdown
}

// workplaceType reads Lever's and Ashby's workplace type: "remote",
// "hybrid", "onsite", "on-site", "OnSite". "unspecified" is none.
func workplaceType(value string) string {
	switch strings.ToLower(strings.ReplaceAll(value, "-", "")) {
	case "remote":
		return domain.RemoteTypeRemote
	case "hybrid":
		return domain.RemoteTypeHybrid
	case "onsite":
		return domain.RemoteTypeOnsite
	}
	return ""
}

func splitLocations(value, separator string) []string {
	return trimAll(strings.Split(value, separator))
}

func trimAll(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			trimmed = append(trimmed, value)
		}
	}
	return trimmed
}

// parseTime is the first of values that is an RFC 3339 time, or a date,
// which Workable gives: "2026-07-30". It is zero when none is.
func parseTime(values ...string) time.Time {
	for _, value := range values {
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed.UTC()
		}
		if parsed, err := time.Parse(time.DateOnly, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
