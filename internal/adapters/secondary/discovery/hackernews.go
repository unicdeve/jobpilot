package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// DefaultHackerNewsBaseURL is the Algolia-hosted Hacker News API, which is
// public and needs no key. See https://hn.algolia.com/api.
const DefaultHackerNewsBaseURL = "https://hn.algolia.com/api/v1"

// hackerNewsCacheTTL is generous because the thread it reads is monthly.
// Comments accrue through the month, so it is not static, but it changes
// far more slowly than a job board.
const hackerNewsCacheTTL = time.Hour

// HackerNewsSource is a ports.JobSource backed by the monthly
// "Ask HN: Who is hiring?" thread.
//
// These postings do not exist on any job board. They are written by the
// hiring team, carry a salary far more often than a board listing does,
// and are the reason this source is worth the parsing it needs.
//
// The thread has a convention rather than a schema:
//
//	Company | Role | Location | Type | Salary | URL
//
// Measured on the September 2026 thread, 92% of the 257 top-level comments
// carry at least two pipes and 80% carry both pipes and a URL. A comment
// that does not fit is skipped rather than guessed at — the same choice
// the browser worker makes with a field whose label it cannot read.
type HackerNewsSource struct {
	baseURL    string
	httpClient *http.Client
	cache      *jobCache[domain.Job]
}

// NewHackerNewsSource constructs a HackerNewsSource targeting baseURL.
func NewHackerNewsSource(baseURL string) *HackerNewsSource {
	return &HackerNewsSource{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		cache:      newJobCache[domain.Job](hackerNewsCacheTTL),
	}
}

// ID returns the source's unique identifier.
func (s *HackerNewsSource) ID() string { return "hackernews" }

// Name returns the source's human-readable name.
func (s *HackerNewsSource) Name() string { return "Hacker News (Who is hiring?)" }

// Capabilities reports search only.
func (s *HackerNewsSource) Capabilities() ports.SourceCapabilities {
	return ports.SourceCapabilities{CanSearch: true, FiltersServerSide: false, CanGetJob: false}
}

type hnSearchResponse struct {
	Hits []struct {
		ObjectID  string `json:"objectID"`
		Title     string `json:"title"`
		CreatedAt string `json:"created_at"`
	} `json:"hits"`
}

type hnItem struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
	Children  []hnItem  `json:"children"`
}

// Search reads the newest hiring thread; JobDiscoveryService applies the
// criteria.
func (s *HackerNewsSource) Search(ctx context.Context, _ domain.SearchCriteria) ([]domain.Job, error) {
	if cached, fresh := s.cache.get(); fresh {
		return cached, nil
	}

	storyID, err := s.latestHiringThread(ctx)
	if err != nil {
		return nil, err
	}

	thread, err := s.fetchItem(ctx, storyID)
	if err != nil {
		return nil, err
	}

	jobs := make([]domain.Job, 0, len(thread.Children))
	for _, comment := range thread.Children {
		if job, ok := s.toDomainJob(comment); ok {
			jobs = append(jobs, job)
		}
	}

	s.cache.put(jobs)
	return jobs, nil
}

// GetJob is unsupported.
func (s *HackerNewsSource) GetJob(context.Context, string) (*domain.Job, error) {
	return nil, fmt.Errorf("hacker news cannot fetch a job by URL")
}

// latestHiringThread finds the newest "Who is hiring?" story.
//
// The same account posts three threads a month — who is hiring, who wants
// to be hired, and freelancer seeking freelancer — so the title is
// checked rather than just taking the newest post by that author. Picking
// the wrong one would fill the user's history with people looking for
// work.
func (s *HackerNewsSource) latestHiringThread(ctx context.Context) (string, error) {
	endpoint := s.baseURL + "/search_by_date?tags=story,author_whoishiring&hitsPerPage=10"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to reach Hacker News: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("hacker news returned non-OK status: %d", resp.StatusCode)
	}

	var parsed hnSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("failed to decode the Hacker News response: %w", err)
	}

	for _, hit := range parsed.Hits {
		if isHiringThreadTitle(hit.Title) {
			return hit.ObjectID, nil
		}
	}
	return "", fmt.Errorf("no 'Who is hiring?' thread found")
}

// isHiringThreadTitle reports whether a title is the hiring thread, as
// opposed to the two sibling threads posted the same day.
func isHiringThreadTitle(title string) bool {
	lower := strings.ToLower(title)
	if !strings.Contains(lower, "who is hiring") {
		return false
	}
	return !strings.Contains(lower, "wants to be hired") && !strings.Contains(lower, "freelancer")
}

func (s *HackerNewsSource) fetchItem(ctx context.Context, id string) (*hnItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/items/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach Hacker News: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hacker news returned non-OK status: %d", resp.StatusCode)
	}

	var item hnItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("failed to decode the Hacker News thread: %w", err)
	}
	return &item, nil
}

var (
	hnURLPattern = regexp.MustCompile(`https?://[^\s<>"')\]]+`)
	// Currency codes need word boundaries. Without them "EUR" matches
	// inside "Europe", and "Remote (Europe)" was read as a salary and
	// dropped from the location.
	hnSalaryPattern = regexp.MustCompile(`(?i)(\$|€|£|\b(USD|EUR|GBP)\b|\d+\s*k\b|\bsalary\b|\bcompensation\b|\bequity\b)`)
	// hnRemoteWords are consumed by RemoteType, so they must not also be
	// stored as a place: "HYBRID" is not somewhere anyone lives.
	hnRemoteWords = regexp.MustCompile(`(?i)^(remote|hybrid|onsite|on-site|in office|in-office|wfh)$`)
)

// toDomainJob parses one comment, reporting whether it could be read.
func (s *HackerNewsSource) toDomainJob(comment hnItem) (domain.Job, bool) {
	text, err := htmlToMarkdown(comment.Text)
	if err != nil || strings.TrimSpace(text) == "" {
		return domain.Job{}, false
	}
	// Entities survive the conversion, so a title reads "Platform &amp;
	// DevOps Engineer" and is then matched, stored and shown that way.
	text = html.UnescapeString(text)

	header := firstNonEmptyLine(text)
	fields := splitHeaderFields(header)
	if len(fields) < 3 {
		// Fewer than "Company | Role | something" is not enough to tell a
		// posting from an ordinary comment.
		return domain.Job{}, false
	}

	// The apply link is required. A job nobody can apply to is a comment,
	// and storing it as a job would put an unusable row in front of the
	// user and hand the browser worker a thread to fill in.
	applyURL := hnURLPattern.FindString(text)
	if applyURL == "" {
		return domain.Job{}, false
	}

	company, title := fields[0], fields[1]
	if company == "" || title == "" {
		return domain.Job{}, false
	}

	job := domain.Job{
		CanonicalURL:        strings.TrimRight(applyURL, ".,);"),
		SourceID:            s.ID(),
		SourceJobID:         strconv.Itoa(comment.ID),
		Title:               title,
		CompanyName:         company,
		DescriptionMarkdown: text,
		RemoteType:          remoteTypeFromFields(fields),
		EmploymentType:      employmentTypeFromFields(fields),
		Locations:           locationsFromFields(fields),
		PostedAt:            comment.CreatedAt,
		DiscoveredAt:        time.Now(),
		// The comment itself, so a reader can always get back to the
		// thread the posting came from.
		RawSourceReference: fmt.Sprintf("https://news.ycombinator.com/item?id=%d", comment.ID),
	}
	return job, true
}

// firstNonEmptyLine returns the header line a posting starts with.
func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// splitHeaderFields splits the convention's pipe-delimited header, and
// reduces each field to plain text.
func splitHeaderFields(header string) []string {
	parts := strings.Split(header, "|")
	fields := make([]string, 0, len(parts))
	for _, part := range parts {
		fields = append(fields, plainHeaderField(part))
	}
	return fields
}

var (
	markdownLinkPattern = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	emptyParensPattern  = regexp.MustCompile(`\(\s*\)|\[\s*\]`)
)

// plainHeaderField is a header field as a person would write it by hand.
//
// The header is read after the comment's HTML has become markdown, and a
// company written with its homepage — "Freeform (http://freeform.co)" —
// arrives as "Freeform ([http://freeform.co](http://freeform.co))". Stored
// as-is, that is the company's name everywhere it is shown. A header field
// is a name, a role or a place, never a link, so links are reduced to their
// text and URLs are dropped; the apply link is read from the whole comment,
// not from here.
func plainHeaderField(field string) string {
	field = markdownLinkPattern.ReplaceAllString(field, "$1")
	field = hnURLPattern.ReplaceAllString(field, "")
	field = emptyParensPattern.ReplaceAllString(field, "")
	return strings.Join(strings.Fields(field), " ")
}

func remoteTypeFromFields(fields []string) string {
	joined := strings.ToLower(strings.Join(fields, " | "))
	switch {
	case strings.Contains(joined, "hybrid"):
		return "hybrid"
	case strings.Contains(joined, "remote"):
		return "remote"
	case strings.Contains(joined, "onsite"), strings.Contains(joined, "on-site"),
		strings.Contains(joined, "in office"), strings.Contains(joined, "in-office"):
		return "onsite"
	}
	return ""
}

func employmentTypeFromFields(fields []string) string {
	for _, field := range fields {
		if normalized := normalizeEmploymentType(field); normalized != "" {
			return normalized
		}
	}
	return ""
}

// locationsFromFields picks the first field that reads as a place.
//
// The convention gives one location, and only the first plausible field is
// taken. Collecting every field that is not obviously something else
// gathered notes as places — "DUTCH REQUIRED" became a second location of
// a Utrecht job — and a wrong location silently hides a job from the
// user's filter, while a missing one leaves it visible. So the ambiguous
// ones are dropped.
func locationsFromFields(fields []string) []string {
	for _, field := range fields[2:] {
		if field == "" || len(field) > 60 {
			continue
		}
		if hnSalaryPattern.MatchString(field) || normalizeEmploymentType(field) != "" {
			continue
		}
		if hnRemoteWords.MatchString(field) {
			continue
		}
		if strings.Contains(strings.ToLower(field), "http") {
			continue
		}
		// A field written in capitals is a note — "DUTCH REQUIRED",
		// "ONSITE ONLY" — not a place name.
		if field == strings.ToUpper(field) && strings.ContainsAny(field, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			continue
		}
		return []string{field}
	}
	return nil
}
