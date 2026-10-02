package discovery

import (
	"context"
	"net/url"
	"strings"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// workableBoard is an account's widget, which lists every published posting
// with its description in one answer (ADR-0026 §1).
type workableBoard struct {
	Name string        `json:"name"`
	Jobs []workableJob `json:"jobs"`
}

type workableJob struct {
	Title     string `json:"title"`
	Shortcode string `json:"shortcode"`
	// Description is the posting's whole text: Workable's own posting
	// endpoint splits it into description, requirements and benefits, and
	// the widget joins them.
	Description    string `json:"description"`
	EmploymentType string `json:"employment_type"`
	Telecommuting  bool   `json:"telecommuting"`
	PublishedOn    string `json:"published_on"`
	CreatedAt      string `json:"created_at"`
	// Locations has one place on every posting measured. A hidden one is
	// not shown on the company's page.
	Locations []struct {
		Country string `json:"country"`
		City    string `json:"city"`
		Region  string `json:"region"`
		Hidden  bool   `json:"hidden"`
	} `json:"locations"`
}

// readWorkable reads an account's widget. An account that does not exist is
// a 404.
func (b *CompanyBoards) readWorkable(ctx context.Context, board domain.Board) ([]domain.Job, string, error) {
	var answer workableBoard
	address := b.endpoints.WorkableAPI + "/api/v1/widget/accounts/" + url.PathEscape(board.Name) + "?details=true"
	if err := b.getJSON(ctx, address, &answer); err != nil {
		return nil, "", err
	}
	jobs := mapJobs(answer.Jobs, func(job workableJob) (domain.Job, bool) { return workableToJob(board, job) })
	return jobs, domain.CleanCompanyName(answer.Name), nil
}

func workableToJob(board domain.Board, item workableJob) (domain.Job, bool) {
	if strings.TrimSpace(item.Title) == "" || item.Shortcode == "" {
		return domain.Job{}, false
	}
	// The widget's own address for a posting, apply.workable.com/j/{code},
	// names no account; this one does, so the job's board can be read from
	// it (ADR-0026 §3).
	address := "https://apply.workable.com/" + url.PathEscape(board.Name) + "/j/" + url.PathEscape(item.Shortcode) + "/"

	var locations []string
	for _, location := range item.Locations {
		if location.Hidden {
			continue
		}
		locations = append(locations, strings.Join(trimAll([]string{location.City, location.Region, location.Country}), ", "))
	}
	job := boardJob(board, item.Shortcode, address, item.Title, describe(item.Description), trimAll(locations))
	// Workable's widget says whether a posting is remote, and nothing more:
	// hybrid and on-site are not told apart.
	if item.Telecommuting {
		job.RemoteType = domain.RemoteTypeRemote
	}
	job.EmploymentType = normalizeEmploymentType(item.EmploymentType)
	job.PostedAt = parseTime(item.PublishedOn, item.CreatedAt)
	return job, true
}
