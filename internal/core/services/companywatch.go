package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// CompanyPagesSourceID is the source that reads the watched companies'
// hiring pages, and the source a job read from one is stored under.
const CompanyPagesSourceID = "company-pages"

// boardsAtOnce is how many hiring pages are read at the same time (spec D5).
const boardsAtOnce = 4

// ErrEmptyCompanyName is returned for a rename to nothing.
var ErrEmptyCompanyName = errors.New("a company needs a name")

// ErrJobNotOnBoard is returned for watching the company of a job whose
// posting is not on a hiring page JobPilot reads.
var ErrJobNotOnBoard = errors.New("this job is not on a Greenhouse, Lever or Ashby hiring page")

// CompanyWatchService keeps the companies the candidate watches and reads
// their hiring pages (docs/02_features/17_company_watchlists.md, ADR-0024).
type CompanyWatchService struct {
	repo   ports.WatchedCompanyRepository
	boards ports.BoardReader
	jobs   ports.JobRepository
	// starter is the starter list; a field so tests can give their own.
	starter func() []domain.StarterCompany
}

// NewCompanyWatchService constructs a CompanyWatchService.
func NewCompanyWatchService(repo ports.WatchedCompanyRepository, boards ports.BoardReader, jobs ports.JobRepository) *CompanyWatchService {
	return &CompanyWatchService{repo: repo, boards: boards, jobs: jobs, starter: domain.StarterCompanies}
}

// Watch adds the company whose hiring page address is on, once its board
// has answered (AC-001). It reports whether the company is new: a board
// already watched returns the company watching it.
func (s *CompanyWatchService) Watch(ctx context.Context, address string) (*domain.WatchedCompany, bool, error) {
	board, err := domain.ParseBoard(address)
	if err != nil {
		return nil, false, err
	}
	return s.watch(ctx, board, address)
}

// WatchJob adds the company whose hiring page a stored job's posting is on.
func (s *CompanyWatchService) WatchJob(ctx context.Context, jobID string) (*domain.WatchedCompany, bool, error) {
	job, err := s.jobs.GetByID(ctx, jobID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get job %s: %w", jobID, ports.AsReference(err))
	}
	board, ok := domain.BoardOf(*job)
	if !ok {
		return nil, false, ErrJobNotOnBoard
	}
	return s.watch(ctx, board, job.CanonicalURL)
}

func (s *CompanyWatchService) watch(ctx context.Context, board domain.Board, addedFrom string) (*domain.WatchedCompany, bool, error) {
	existing, err := s.repo.GetByBoard(ctx, board)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, false, nil
	}

	// Checked once, so a mistyped address is refused rather than watched.
	contents, err := s.boards.ReadBoard(ctx, board)
	if err != nil {
		return nil, false, fmt.Errorf("could not read %s: %w", board.URL(), err)
	}
	name, err := s.boards.BoardName(ctx, board)
	if errors.Is(err, ports.ErrBoardGone) {
		return nil, false, fmt.Errorf("could not read %s: %w", board.URL(), err)
	}
	if err != nil {
		// The name is a convenience; the board answered, which is what
		// watching needs.
		slog.WarnContext(ctx, "could not read a hiring page's company name", "board", board.URL(), "error", err)
	}
	if name = domain.CleanCompanyName(name); name == "" {
		name = board.DefaultName()
	}

	company, err := s.repo.Create(ctx, &domain.WatchedCompany{
		Board: board, Name: name, AddedFrom: addedFrom, AddedAt: time.Now().UTC(),
	})
	if err != nil {
		return nil, false, err
	}
	if err := s.repo.RecordCheck(ctx, company.ID, contents.FetchedAt, domain.BoardCheckOK, len(contents.Jobs)); err != nil {
		return nil, false, err
	}
	company, err = s.repo.GetByID(ctx, company.ID)
	return company, true, err
}

// List returns the watched companies, by name.
func (s *CompanyWatchService) List(ctx context.Context) ([]*domain.WatchedCompany, error) {
	return s.repo.List(ctx)
}

// Rename gives a watched company the name the candidate calls it.
func (s *CompanyWatchService) Rename(ctx context.Context, id, name string) (*domain.WatchedCompany, error) {
	if name = domain.CleanCompanyName(name); name == "" {
		return nil, ErrEmptyCompanyName
	}
	if err := s.repo.Rename(ctx, id, name); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, id)
}

// Unwatch stops reading a company's page. Its jobs stay, as a deleted
// saved search's do.
func (s *CompanyWatchService) Unwatch(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// Suggestions is the starter list, less the companies already watched
// (AC-007).
func (s *CompanyWatchService) Suggestions(ctx context.Context) ([]domain.StarterCompany, error) {
	watched, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	suggestions := make([]domain.StarterCompany, 0)
	for _, suggestion := range s.starter() {
		if !watching(watched, suggestion.Board) {
			suggestions = append(suggestions, suggestion)
		}
	}
	return suggestions, nil
}

func watching(companies []*domain.WatchedCompany, board domain.Board) bool {
	for _, company := range companies {
		if company.Board.Same(board) {
			return true
		}
	}
	return false
}

// JobCompany is the hiring page a job's posting is on, and the watched
// company it is, if any (AC-002).
type JobCompany struct {
	Board   *domain.Board          `json:"board"`
	URL     string                 `json:"url,omitempty"`
	Company *domain.WatchedCompany `json:"company"`
}

// CompanyForJob returns the hiring page a stored job is on.
func (s *CompanyWatchService) CompanyForJob(ctx context.Context, jobID string) (*JobCompany, error) {
	job, err := s.jobs.GetByID(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job %s: %w", jobID, err)
	}
	board, ok := domain.BoardOf(*job)
	if !ok {
		return &JobCompany{}, nil
	}
	company, err := s.repo.GetByBoard(ctx, board)
	if err != nil {
		return nil, err
	}
	return &JobCompany{Board: &board, URL: board.URL(), Company: company}, nil
}

// Source is the watched companies as one job source, "Company pages"
// (ADR-0024 §3).
func (s *CompanyWatchService) Source() ports.JobSource {
	return companyPagesSource{s}
}

type companyPagesSource struct{ svc *CompanyWatchService }

func (companyPagesSource) ID() string   { return CompanyPagesSourceID }
func (companyPagesSource) Name() string { return "Company pages" }

// Capabilities reports search only. Discovery applies the criteria, as it
// does for every board (spec D3).
func (companyPagesSource) Capabilities() ports.SourceCapabilities {
	return ports.SourceCapabilities{CanSearch: true, FiltersServerSide: false}
}

func (companyPagesSource) GetJob(context.Context, string) (*domain.Job, error) {
	return nil, errors.New("company pages cannot fetch a job by URL")
}

// boardRead is what reading one company's page found.
type boardRead struct {
	contents *domain.BoardContents
	err      error
}

// Search reads every watched company's page, at most four at a time, and
// records how each went. A company whose page did not answer is named, and
// the others' postings are kept (AC-005).
func (source companyPagesSource) Search(ctx context.Context, _ domain.SearchCriteria) ([]domain.Job, error) {
	s := source.svc
	companies, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the watched companies: %w", err)
	}

	reads := make([]boardRead, len(companies))
	slots := make(chan struct{}, boardsAtOnce)
	var wg sync.WaitGroup
	for i, company := range companies {
		wg.Go(func() {
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				reads[i] = boardRead{err: ctx.Err()}
				return
			}
			defer func() { <-slots }()
			contents, err := s.boards.ReadBoard(ctx, company.Board)
			reads[i] = boardRead{contents: contents, err: err}
		})
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var jobs []domain.Job
	var failing []string
	unreachable := 0
	for i, company := range companies {
		read := reads[i]
		if read.err != nil {
			failing = append(failing, company.Name)
			check := checkOf(read.err)
			if check == domain.BoardCheckUnreachable {
				unreachable++
			}
			slog.WarnContext(ctx, "watched company's page did not answer", "company", company.Name, "check", check, "error", read.err)
			openRoles := company.OpenRoles
			if check == domain.BoardCheckGone {
				openRoles = 0
			}
			s.record(ctx, company, time.Now().UTC(), check, openRoles)
			continue
		}
		s.record(ctx, company, read.contents.FetchedAt, domain.BoardCheckOK, len(read.contents.Jobs))
		for _, job := range read.contents.Jobs {
			if job.CompanyName == "" {
				job.CompanyName = company.Name
			}
			jobs = append(jobs, job)
		}
	}

	switch {
	case len(failing) == 0:
		return jobs, nil
	case len(failing) < len(companies):
		return jobs, &ports.PartialAnswer{Failing: failing}
	case unreachable == len(companies):
		return nil, &ports.PartialAnswer{Failing: failing, Err: fmt.Errorf("no watched company's page answered: %w", ports.ErrSourceUnreachable)}
	default:
		return nil, &ports.PartialAnswer{Failing: failing, Err: errors.New("no watched company's page answered")}
	}
}

// record stores a check. Failing to is logged, not the search's failure.
func (s *CompanyWatchService) record(ctx context.Context, company *domain.WatchedCompany, at time.Time, check domain.BoardCheck, openRoles int) {
	if err := s.repo.RecordCheck(ctx, company.ID, at, check, openRoles); err != nil && !errors.Is(err, ports.ErrWatchedCompanyNotFound) {
		slog.WarnContext(ctx, "could not record a watched company's check", "company", company.Name, "error", err)
	}
}

// checkOf is the check a failed read is recorded as.
func checkOf(err error) domain.BoardCheck {
	switch {
	case errors.Is(err, ports.ErrBoardGone):
		return domain.BoardCheckGone
	case errors.Is(err, ports.ErrSourceUnreachable):
		return domain.BoardCheckUnreachable
	}
	return domain.BoardCheckFailed
}
