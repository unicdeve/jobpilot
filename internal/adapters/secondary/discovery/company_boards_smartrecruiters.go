package discovery

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// smartRecruitersPageSize is the most postings SmartRecruiters' list returns
// at once. Bosch's 4,822, measured on 2026-10-02, are 49 requests.
const smartRecruitersPageSize = 100

// postingsAtOnce bounds how many postings of one board are described at
// once. SmartRecruiters answered 120 in a row with no sign of a limit.
const postingsAtOnce = 4

type smartRecruitersList struct {
	TotalFound int                      `json:"totalFound"`
	Content    []smartRecruitersPosting `json:"content"`
}

type smartRecruitersPosting struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Company struct {
		Name string `json:"name"`
	} `json:"company"`
	ReleasedDate string `json:"releasedDate"`
	// Location is the posting's one place. A remote posting names a city
	// too: Bosch's "AI and Cloud Development Engineer - Remote" is in
	// Timișoara (ADR-0026 §4).
	Location struct {
		Remote       bool   `json:"remote"`
		Hybrid       bool   `json:"hybrid"`
		FullLocation string `json:"fullLocation"`
	} `json:"location"`
	TypeOfEmployment struct {
		Label string `json:"label"`
	} `json:"typeOfEmployment"`
}

// smartRecruitersDetails is one posting as its own endpoint gives it, the
// only place its description is.
type smartRecruitersDetails struct {
	JobAd struct {
		Sections struct {
			CompanyDescription    smartRecruitersSection `json:"companyDescription"`
			JobDescription        smartRecruitersSection `json:"jobDescription"`
			Qualifications        smartRecruitersSection `json:"qualifications"`
			AdditionalInformation smartRecruitersSection `json:"additionalInformation"`
		} `json:"sections"`
	} `json:"jobAd"`
}

type smartRecruitersSection struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

// readSmartRecruiters lists a company's postings, a page at a time. The list
// carries no descriptions, so the board is Undescribed.
//
// SmartRecruiters answers a company that does not exist as one with no
// postings, so an empty list is checked against the careers page.
func (b *CompanyBoards) readSmartRecruiters(ctx context.Context, board domain.Board) ([]domain.Job, string, error) {
	var postings []smartRecruitersPosting
	for offset := 0; ; offset += smartRecruitersPageSize {
		var page smartRecruitersList
		address := fmt.Sprintf("%s/v1/companies/%s/postings?limit=%d&offset=%d",
			b.endpoints.SmartRecruitersAPI, url.PathEscape(board.Name), smartRecruitersPageSize, offset)
		if err := b.getJSON(ctx, address, &page); err != nil {
			return nil, "", err
		}
		postings = append(postings, page.Content...)
		if len(page.Content) == 0 || offset+smartRecruitersPageSize >= page.TotalFound {
			break
		}
	}
	if len(postings) == 0 {
		name, err := b.smartRecruitersPageName(ctx, board)
		return nil, name, err
	}
	name := domain.CleanCompanyName(postings[0].Company.Name)
	return mapJobs(postings, func(posting smartRecruitersPosting) (domain.Job, bool) {
		return smartRecruitersToJob(board, posting)
	}), name, nil
}

// smartRecruitersPageName reads a company's careers page, which says whether
// the company exists: one that does not is redirected to SmartRecruiters'
// search across every company. One that does is a page titled "Careers at
// Sectigo", or a redirect to the company's own careers site, which names
// nothing here (ADR-0026 §1).
func (b *CompanyBoards) smartRecruitersPageName(ctx context.Context, board domain.Board) (string, error) {
	page, location, err := b.getOnce(ctx, b.endpoints.SmartRecruitersPages+"/"+url.PathEscape(board.Name))
	if err != nil {
		return "", err
	}
	if location != "" {
		target, err := url.Parse(location)
		if err == nil && strings.EqualFold(target.Hostname(), "jobs.smartrecruiters.com") && strings.Trim(target.Path, "/") == "" {
			return "", ports.ErrBoardGone
		}
		return "", nil
	}
	match := pageTitle.FindSubmatch(page)
	if match == nil {
		return "", nil
	}
	title := html.UnescapeString(string(match[1]))
	name, ok := strings.CutPrefix(strings.TrimSpace(title), "Careers at ")
	if !ok {
		return "", nil
	}
	return domain.CleanCompanyName(name), nil
}

func smartRecruitersToJob(board domain.Board, posting smartRecruitersPosting) (domain.Job, bool) {
	if posting.ID == "" || strings.TrimSpace(posting.Name) == "" {
		return domain.Job{}, false
	}
	// The list gives no posting's address; this one is the posting's page,
	// without the title's slug, which SmartRecruiters does not require.
	address := "https://jobs.smartrecruiters.com/" + url.PathEscape(board.Name) + "/" + url.PathEscape(posting.ID)
	job := boardJob(board, posting.ID, address, posting.Name, "", trimAll([]string{posting.Location.FullLocation}))
	job.CompanyName = strings.TrimSpace(posting.Company.Name)
	switch {
	case posting.Location.Remote:
		job.RemoteType = domain.RemoteTypeRemote
	case posting.Location.Hybrid:
		job.RemoteType = domain.RemoteTypeHybrid
	}
	job.EmploymentType = normalizeEmploymentType(posting.TypeOfEmployment.Label)
	job.PostedAt = parseTime(posting.ReleasedDate)
	return job, true
}

// describedPosting is a posting's description, kept by the posting's ID and
// release so a later read describes only what is new or re-released.
type describedPosting struct {
	released    string
	description string
}

// descriptions are the postings described so far, per board. A board's are
// dropped when it is next read without them, so the cache holds no more than
// the boards' open postings.
type descriptions struct {
	mu      sync.Mutex
	byBoard map[string]map[string]describedPosting
}

func (d *descriptions) get(board, posting, released string) (string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	kept, ok := d.byBoard[board][posting]
	if !ok || kept.released != released {
		return "", false
	}
	return kept.description, true
}

func (d *descriptions) put(board, posting, released, description string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.byBoard[board] == nil {
		d.byBoard[board] = make(map[string]describedPosting)
	}
	d.byBoard[board][posting] = describedPosting{released: released, description: description}
}

// keepOnly forgets a board's descriptions of postings it no longer lists.
func (d *descriptions) keepOnly(board string, jobs []domain.Job) {
	d.mu.Lock()
	defer d.mu.Unlock()
	kept := d.byBoard[board]
	if kept == nil {
		return
	}
	listed := make(map[string]bool, len(jobs))
	for _, job := range jobs {
		listed[postingID(job)] = true
	}
	for id := range kept {
		if !listed[id] {
			delete(kept, id)
		}
	}
}

// postingID is the provider's ID of the posting a board job was read from:
// the last part of its SourceJobID, provider/board/id.
func postingID(job domain.Job) string {
	return job.SourceJobID[strings.LastIndex(job.SourceJobID, "/")+1:]
}

// DescribeJobs reads each job's description from its own posting, at most
// postingsAtOnce at a time, unless it was read before and the posting has
// not been re-released since.
func (b *CompanyBoards) DescribeJobs(ctx context.Context, board domain.Board, jobs []domain.Job) ([]domain.Job, error) {
	if board.Provider != domain.ProviderSmartRecruiters {
		return jobs, nil
	}
	key := boardKey(board)
	described := make([]domain.Job, len(jobs))
	gone := make([]bool, len(jobs))
	errs := make([]error, len(jobs))
	slots := make(chan struct{}, postingsAtOnce)
	var wg sync.WaitGroup
	for i, job := range jobs {
		id := postingID(job)
		released := strconv.FormatInt(job.PostedAt.UnixMilli(), 10)
		if description, ok := b.descriptions.get(key, id, released); ok {
			job.DescriptionMarkdown = description
			described[i] = job
			continue
		}
		wg.Go(func() {
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			}
			defer func() { <-slots }()
			description, err := b.describeSmartRecruiters(ctx, board, id)
			switch {
			case errors.Is(err, ports.ErrBoardGone):
				// Closed between the list and now.
				gone[i] = true
			case err != nil:
				errs[i] = err
			default:
				b.descriptions.put(key, id, released, description)
				job.DescriptionMarkdown = description
				described[i] = job
			}
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("failed to read the postings' descriptions: %w", err)
	}
	kept := described[:0]
	for i, job := range described {
		if !gone[i] {
			kept = append(kept, job)
		}
	}
	return kept, nil
}

// describeSmartRecruiters reads one posting's sections into one description:
// the company, the job, its qualifications and anything else it adds.
func (b *CompanyBoards) describeSmartRecruiters(ctx context.Context, board domain.Board, id string) (string, error) {
	var details smartRecruitersDetails
	address := fmt.Sprintf("%s/v1/companies/%s/postings/%s", b.endpoints.SmartRecruitersAPI, url.PathEscape(board.Name), url.PathEscape(id))
	if err := b.getJSON(ctx, address, &details); err != nil {
		return "", err
	}
	sections := details.JobAd.Sections
	var page strings.Builder
	for _, section := range []smartRecruitersSection{
		sections.CompanyDescription, sections.JobDescription, sections.Qualifications, sections.AdditionalInformation,
	} {
		if strings.TrimSpace(section.Text) == "" {
			continue
		}
		if section.Title != "" {
			fmt.Fprintf(&page, "<h3>%s</h3>", html.EscapeString(section.Title))
		}
		page.WriteString(section.Text)
	}
	return describe(page.String()), nil
}
