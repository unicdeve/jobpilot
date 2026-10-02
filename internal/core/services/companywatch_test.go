package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// fakeWatchedCompanyRepository is an in-memory
// ports.WatchedCompanyRepository.
type fakeWatchedCompanyRepository struct {
	mu        sync.Mutex
	companies map[string]*domain.WatchedCompany
}

func newFakeWatchedCompanyRepository() *fakeWatchedCompanyRepository {
	return &fakeWatchedCompanyRepository{companies: map[string]*domain.WatchedCompany{}}
}

func (f *fakeWatchedCompanyRepository) Create(_ context.Context, company *domain.WatchedCompany) (*domain.WatchedCompany, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.companies {
		if existing.Board.Same(company.Board) {
			copied := *existing
			return &copied, nil
		}
	}
	company.ID = fmt.Sprintf("company-%d", len(f.companies)+1)
	company.URL = company.Board.URL()
	stored := *company
	f.companies[company.ID] = &stored
	copied := stored
	return &copied, nil
}

func (f *fakeWatchedCompanyRepository) GetByID(_ context.Context, id string) (*domain.WatchedCompany, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	company, ok := f.companies[id]
	if !ok {
		return nil, ports.ErrWatchedCompanyNotFound
	}
	copied := *company
	return &copied, nil
}

func (f *fakeWatchedCompanyRepository) GetByBoard(_ context.Context, board domain.Board) (*domain.WatchedCompany, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, company := range f.companies {
		if company.Board.Same(board) {
			copied := *company
			return &copied, nil
		}
	}
	return nil, nil
}

func (f *fakeWatchedCompanyRepository) List(_ context.Context) ([]*domain.WatchedCompany, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	companies := make([]*domain.WatchedCompany, 0, len(f.companies))
	for _, company := range f.companies {
		copied := *company
		companies = append(companies, &copied)
	}
	sort.Slice(companies, func(i, j int) bool { return companies[i].Name < companies[j].Name })
	return companies, nil
}

func (f *fakeWatchedCompanyRepository) Rename(_ context.Context, id, name string) error {
	return f.update(id, func(company *domain.WatchedCompany) { company.Name = name })
}

func (f *fakeWatchedCompanyRepository) RecordCheck(_ context.Context, id string, at time.Time, check domain.BoardCheck, openRoles int) error {
	return f.update(id, func(company *domain.WatchedCompany) {
		company.CheckedAt, company.Check, company.OpenRoles = &at, check, openRoles
	})
}

func (f *fakeWatchedCompanyRepository) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.companies[id]; !ok {
		return ports.ErrWatchedCompanyNotFound
	}
	delete(f.companies, id)
	return nil
}

func (f *fakeWatchedCompanyRepository) update(id string, change func(*domain.WatchedCompany)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	company, ok := f.companies[id]
	if !ok {
		return ports.ErrWatchedCompanyNotFound
	}
	change(company)
	return nil
}

// fakeBoardReader answers each board by its name: its jobs, or its error.
type fakeBoardReader struct {
	jobs    map[string][]domain.Job
	errs    map[string]error
	names   map[string]string
	nameErr error

	// undescribed boards list their postings without descriptions, which
	// DescribeJobs adds from descriptions, by title.
	undescribed  map[string]bool
	descriptions map[string]string
	describeErr  error
	describedMu  sync.Mutex
	described    []string
	// resolved is where each short address leads.
	resolved map[string]string

	inFlight, most atomic.Int32
	reads          atomic.Int32
	delay          time.Duration
}

func (f *fakeBoardReader) ReadBoard(_ context.Context, board domain.Board) (*domain.BoardContents, error) {
	f.reads.Add(1)
	now := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for {
		most := f.most.Load()
		if now <= most || f.most.CompareAndSwap(most, now) {
			break
		}
	}
	time.Sleep(f.delay)
	if err := f.errs[board.Name]; err != nil {
		return nil, err
	}
	return &domain.BoardContents{
		Jobs: f.jobs[board.Name], FetchedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC), Undescribed: f.undescribed[board.Name],
	}, nil
}

func (f *fakeBoardReader) DescribeJobs(_ context.Context, _ domain.Board, jobs []domain.Job) ([]domain.Job, error) {
	if f.describeErr != nil {
		return nil, f.describeErr
	}
	described := make([]domain.Job, len(jobs))
	for i, job := range jobs {
		f.describedMu.Lock()
		f.described = append(f.described, job.Title)
		f.describedMu.Unlock()
		job.DescriptionMarkdown = f.descriptions[job.Title]
		described[i] = job
	}
	return described, nil
}

func (f *fakeBoardReader) ResolveAddress(_ context.Context, address string) (string, error) {
	resolved, ok := f.resolved[address]
	if !ok {
		return "", ports.ErrBoardGone
	}
	return resolved, nil
}

func (f *fakeBoardReader) BoardName(_ context.Context, board domain.Board) (string, error) {
	if f.nameErr != nil {
		return "", f.nameErr
	}
	return f.names[board.Name], nil
}

func boardJobs(board string, titles ...string) []domain.Job {
	jobs := make([]domain.Job, len(titles))
	for i, title := range titles {
		jobs[i] = domain.Job{
			CanonicalURL: fmt.Sprintf("https://job-boards.greenhouse.io/%s/jobs/%d", board, i),
			SourceID:     CompanyPagesSourceID,
			Title:        title,
			Locations:    []string{"Remote, Worldwide"},
			RemoteType:   domain.RemoteTypeRemote,
		}
	}
	return jobs
}

func newCompanyWatch(boards *fakeBoardReader) (*CompanyWatchService, *fakeWatchedCompanyRepository, *fakeJobRepository) {
	repo := newFakeWatchedCompanyRepository()
	jobs := newFakeJobRepository()
	return NewCompanyWatchService(repo, boards, jobs), repo, jobs
}

func TestCompanyWatchService_Watch(t *testing.T) {
	t.Parallel()
	boards := &fakeBoardReader{
		jobs:  map[string][]domain.Job{"gitlab": boardJobs("gitlab", "AI Engineer", "Backend Engineer")},
		names: map[string]string{"gitlab": "  GitLab\n"},
	}
	svc, _, _ := newCompanyWatch(boards)
	ctx := context.Background()

	company, created, err := svc.Watch(ctx, "https://job-boards.greenhouse.io/gitlab/jobs/8556658002")
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "GitLab", company.Name, "named from the board, as plain text")
	assert.Equal(t, domain.Board{Provider: domain.ProviderGreenhouse, Name: "gitlab"}, company.Board)
	assert.Equal(t, "https://job-boards.greenhouse.io/gitlab/jobs/8556658002", company.AddedFrom)
	assert.Equal(t, domain.BoardCheckOK, company.Check)
	assert.Equal(t, 2, company.OpenRoles)

	again, created, err := svc.Watch(ctx, "boards.greenhouse.io/GitLab")
	require.NoError(t, err)
	assert.False(t, created, "a board is watched once")
	assert.Equal(t, company.ID, again.ID)
	assert.Equal(t, int32(1), boards.reads.Load(), "a board already watched is not read again")
}

func TestCompanyWatchService_Watch_Refusals(t *testing.T) {
	t.Parallel()
	boards := &fakeBoardReader{errs: map[string]error{"closed": ports.ErrBoardGone}}
	svc, repo, _ := newCompanyWatch(boards)
	ctx := context.Background()

	_, _, err := svc.Watch(ctx, "https://www.linkedin.com/company/gitlab")
	require.ErrorIs(t, err, domain.ErrUnsupportedAddress)
	assert.Contains(t, err.Error(), "jobs.lever.co", "the refusal says what is supported")
	assert.Zero(t, boards.reads.Load(), "an unsupported address is not fetched")

	_, _, err = svc.Watch(ctx, "jobs.ashbyhq.com/closed")
	assert.ErrorIs(t, err, ports.ErrBoardGone)

	companies, _ := repo.List(ctx)
	assert.Empty(t, companies)
}

func TestCompanyWatchService_Watch_Names(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("a board that gives no name is called by its own", func(t *testing.T) {
		t.Parallel()
		svc, _, _ := newCompanyWatch(&fakeBoardReader{})
		company, _, err := svc.Watch(ctx, "jobs.ashbyhq.com/grafana-labs")
		require.NoError(t, err)
		assert.Equal(t, "Grafana labs", company.Name)
	})

	t.Run("a page that could not be read for its name is still watched", func(t *testing.T) {
		t.Parallel()
		svc, _, _ := newCompanyWatch(&fakeBoardReader{nameErr: errors.New("status 500")})
		company, _, err := svc.Watch(ctx, "jobs.lever.co/spotify")
		require.NoError(t, err)
		assert.Equal(t, "Spotify", company.Name)
	})

	t.Run("a Lever page that says 404 is a board that has gone", func(t *testing.T) {
		t.Parallel()
		svc, repo, _ := newCompanyWatch(&fakeBoardReader{nameErr: ports.ErrBoardGone})
		_, _, err := svc.Watch(ctx, "jobs.lever.co/kraken")
		require.ErrorIs(t, err, ports.ErrBoardGone)
		companies, _ := repo.List(ctx)
		assert.Empty(t, companies)
	})
}

// A Workable posting's short address names no account, and is followed to
// the one that does (ADR-0026 §3).
func TestCompanyWatchService_Watch_WorkableShortAddress(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	boards := &fakeBoardReader{
		names:    map[string]string{"huggingface": "Hugging Face"},
		resolved: map[string]string{"https://apply.workable.com/j/F4C096B22E": "https://apply.workable.com/huggingface/j/F4C096B22E"},
	}
	svc, _, _ := newCompanyWatch(boards)

	company, created, err := svc.Watch(ctx, "https://apply.workable.com/j/F4C096B22E")
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, domain.Board{Provider: domain.ProviderWorkable, Name: "huggingface"}, company.Board)
	assert.Equal(t, "Hugging Face", company.Name)
	assert.Equal(t, "https://apply.workable.com/j/F4C096B22E", company.AddedFrom, "it was added from what the candidate gave")

	_, _, err = svc.Watch(ctx, "https://apply.workable.com/j/0000000000")
	assert.ErrorIs(t, err, ports.ErrBoardGone, "a posting Workable does not have names no company")
}

func TestCompanyWatchService_WatchJob(t *testing.T) {
	t.Parallel()
	svc, _, jobs := newCompanyWatch(&fakeBoardReader{names: map[string]string{"clickup": "ClickUp"}})
	ctx := context.Background()
	onBoard := &domain.Job{CanonicalURL: "https://news.ycombinator.com/item?id=1",
		DescriptionMarkdown: "Apply at [our page](https://jobs.ashbyhq.com/clickup/1e8c7f3d/application)."}
	elsewhere := &domain.Job{CanonicalURL: "https://remoteok.com/remote-jobs/1", DescriptionMarkdown: "Apply by email."}
	require.NoError(t, jobs.Save(ctx, onBoard))
	require.NoError(t, jobs.Save(ctx, elsewhere))

	found, err := svc.CompanyForJob(ctx, onBoard.ID)
	require.NoError(t, err)
	require.NotNil(t, found.Board)
	assert.Equal(t, "https://jobs.ashbyhq.com/clickup", found.URL)
	assert.Nil(t, found.Company, "not watched yet, so the job offers to")

	company, created, err := svc.WatchJob(ctx, onBoard.ID)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "ClickUp", company.Name)
	assert.Equal(t, onBoard.CanonicalURL, company.AddedFrom)

	found, err = svc.CompanyForJob(ctx, onBoard.ID)
	require.NoError(t, err)
	require.NotNil(t, found.Company, "watched now, so the job does not offer again (AC-002)")
	assert.Equal(t, company.ID, found.Company.ID)

	none, err := svc.CompanyForJob(ctx, elsewhere.ID)
	require.NoError(t, err)
	assert.Nil(t, none.Board)
	_, _, err = svc.WatchJob(ctx, elsewhere.ID)
	assert.ErrorIs(t, err, ErrJobNotOnBoard)
}

func TestCompanyWatchService_RenameAndUnwatch(t *testing.T) {
	t.Parallel()
	svc, _, _ := newCompanyWatch(&fakeBoardReader{})
	ctx := context.Background()
	company, _, err := svc.Watch(ctx, "jobs.lever.co/spotify")
	require.NoError(t, err)

	renamed, err := svc.Rename(ctx, company.ID, " Spotify  AB ")
	require.NoError(t, err)
	assert.Equal(t, "Spotify AB", renamed.Name)

	_, err = svc.Rename(ctx, company.ID, "   ")
	assert.ErrorIs(t, err, ErrEmptyCompanyName)
	_, err = svc.Rename(ctx, "nope", "x")
	assert.ErrorIs(t, err, ports.ErrWatchedCompanyNotFound)

	require.NoError(t, svc.Unwatch(ctx, company.ID))
	assert.ErrorIs(t, svc.Unwatch(ctx, company.ID), ports.ErrWatchedCompanyNotFound)
}

func TestCompanyWatchService_Suggestions(t *testing.T) {
	t.Parallel()
	svc, _, _ := newCompanyWatch(&fakeBoardReader{})
	svc.starter = func() []domain.StarterCompany {
		return []domain.StarterCompany{
			{Board: domain.Board{Provider: domain.ProviderGreenhouse, Name: "gitlab"}, Name: "GitLab"},
			{Board: domain.Board{Provider: domain.ProviderAshby, Name: "railway"}, Name: "Railway"},
		}
	}
	ctx := context.Background()
	_, _, err := svc.Watch(ctx, "boards.greenhouse.io/GITLAB")
	require.NoError(t, err)

	suggestions, err := svc.Suggestions(ctx)
	require.NoError(t, err)
	require.Len(t, suggestions, 1, "a company already watched is not suggested")
	assert.Equal(t, "Railway", suggestions[0].Name)
}

func watchAll(t *testing.T, svc *CompanyWatchService, names ...string) {
	t.Helper()
	for _, name := range names {
		_, _, err := svc.Watch(context.Background(), "boards.greenhouse.io/"+name)
		require.NoError(t, err)
	}
}

func TestCompanyPagesSource_ReadsEveryCompany(t *testing.T) {
	t.Parallel()
	boards := &fakeBoardReader{
		jobs: map[string][]domain.Job{
			"gitlab": boardJobs("gitlab", "AI Engineer"),
			"acme":   boardJobs("acme", "Backend Engineer", "Sales"),
		},
		names: map[string]string{"gitlab": "GitLab", "acme": "Acme"},
	}
	svc, repo, _ := newCompanyWatch(boards)
	watchAll(t, svc, "gitlab", "acme")
	ctx := context.Background()

	jobs, err := svc.Source().Search(ctx, domain.SearchCriteria{Title: "engineer"})
	require.NoError(t, err)
	assert.Len(t, jobs, 3, "every posting: discovery applies the criteria")
	for _, job := range jobs {
		assert.NotEmpty(t, job.CompanyName, "a posting without a company name is given the watched company's")
	}

	companies, _ := repo.List(ctx)
	for _, company := range companies {
		assert.Equal(t, domain.BoardCheckOK, company.Check)
		require.NotNil(t, company.CheckedAt)
		assert.Equal(t, time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC), *company.CheckedAt, "checked when the board answered")
	}
}

// SmartRecruiters lists postings without descriptions and describes each
// with a request of its own, so only the postings a search may keep are
// described; the board's open roles still count every posting (ADR-0026 §2).
func TestCompanyPagesSource_DescribesOnlyWhatMayMatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	newBoards := func() *fakeBoardReader {
		return &fakeBoardReader{
			jobs:         map[string][]domain.Job{"Sectigo": boardJobs("Sectigo", "Revenue Accountant", "Backend Engineer", "Platform Engineer")},
			names:        map[string]string{"Sectigo": "Sectigo"},
			undescribed:  map[string]bool{"Sectigo": true},
			descriptions: map[string]string{"Backend Engineer": "Go and Postgres.", "Platform Engineer": "Kubernetes."},
		}
	}

	t.Run("described after the criteria that need no description", func(t *testing.T) {
		t.Parallel()
		boards := newBoards()
		svc, repo, _ := newCompanyWatch(boards)
		_, _, err := svc.Watch(ctx, "careers.smartrecruiters.com/Sectigo")
		require.NoError(t, err)

		jobs, err := svc.Source().Search(ctx, domain.SearchCriteria{Title: "engineer", Keywords: "postgres"})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"Backend Engineer", "Platform Engineer"}, boards.described,
			"the accountant cannot match a title of engineer, so it is not described")
		require.Len(t, jobs, 2, "keywords are discovery's to apply, once described")
		assert.Equal(t, "Go and Postgres.", jobs[0].DescriptionMarkdown)

		companies, _ := repo.List(ctx)
		assert.Equal(t, 3, companies[0].OpenRoles, "open roles are the board's, before any search")
	})

	t.Run("a board whose postings could not be described is named as failing", func(t *testing.T) {
		t.Parallel()
		boards := newBoards()
		svc, repo, _ := newCompanyWatch(boards)
		_, _, err := svc.Watch(ctx, "careers.smartrecruiters.com/Sectigo")
		require.NoError(t, err)
		boards.describeErr = fmt.Errorf("dial: %w", ports.ErrSourceUnreachable)

		_, err = svc.Source().Search(ctx, domain.SearchCriteria{Title: "engineer"})
		var partial *ports.PartialAnswer
		require.ErrorAs(t, err, &partial)
		assert.Equal(t, []string{"Sectigo"}, partial.Failing)
		companies, _ := repo.List(ctx)
		assert.Equal(t, domain.BoardCheckUnreachable, companies[0].Check)
	})

	t.Run("nothing is described when nothing may match", func(t *testing.T) {
		t.Parallel()
		boards := newBoards()
		svc, _, _ := newCompanyWatch(boards)
		_, _, err := svc.Watch(ctx, "careers.smartrecruiters.com/Sectigo")
		require.NoError(t, err)

		jobs, err := svc.Source().Search(ctx, domain.SearchCriteria{Title: "designer"})
		require.NoError(t, err)
		assert.Empty(t, jobs)
		assert.Empty(t, boards.described)
	})
}

func TestCompanyPagesSource_NoneWatched(t *testing.T) {
	t.Parallel()
	svc, _, _ := newCompanyWatch(&fakeBoardReader{})

	jobs, err := svc.Source().Search(context.Background(), domain.SearchCriteria{Title: "engineer"})
	require.NoError(t, err)
	assert.Empty(t, jobs)
}

func TestCompanyPagesSource_ReadsFourAtATime(t *testing.T) {
	t.Parallel()
	boards := &fakeBoardReader{delay: 20 * time.Millisecond}
	svc, _, _ := newCompanyWatch(boards)
	names := make([]string, 10)
	for i := range names {
		names[i] = fmt.Sprintf("company%d", i)
	}
	watchAll(t, svc, names...)
	boards.most.Store(0)

	_, err := svc.Source().Search(context.Background(), domain.SearchCriteria{Title: "engineer"})
	require.NoError(t, err)
	assert.Equal(t, int32(4), boards.most.Load())
}

func TestCompanyPagesSource_NamesTheCompaniesThatFail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	unreachable := fmt.Errorf("dial: %w", ports.ErrSourceUnreachable)

	t.Run("some failing is a partial answer, and each is recorded", func(t *testing.T) {
		t.Parallel()
		boards := &fakeBoardReader{
			jobs:  map[string][]domain.Job{"gitlab": boardJobs("gitlab", "AI Engineer")},
			names: map[string]string{"gitlab": "GitLab", "closed": "Closed Co", "down": "Down Co"},
		}
		svc, repo, _ := newCompanyWatch(boards)
		watchAll(t, svc, "gitlab", "closed", "down")
		boards.errs = map[string]error{"closed": ports.ErrBoardGone, "down": unreachable}

		jobs, err := svc.Source().Search(ctx, domain.SearchCriteria{Title: "engineer"})
		assert.Len(t, jobs, 1)
		var partial *ports.PartialAnswer
		require.ErrorAs(t, err, &partial)
		assert.Equal(t, []string{"Closed Co", "Down Co"}, partial.Failing)
		assert.NoError(t, partial.Err, "a company answered, so the source did")

		companies, _ := repo.List(ctx)
		checks := map[string]domain.BoardCheck{}
		roles := map[string]int{}
		for _, company := range companies {
			checks[company.Name], roles[company.Name] = company.Check, company.OpenRoles
		}
		assert.Equal(t, map[string]domain.BoardCheck{
			"Closed Co": domain.BoardCheckGone, "Down Co": domain.BoardCheckUnreachable, "GitLab": domain.BoardCheckOK,
		}, checks, "a company that failed stays on the list (AC-005)")
		assert.Zero(t, roles["Closed Co"])
	})

	t.Run("every company unreachable is the source unreachable", func(t *testing.T) {
		t.Parallel()
		boards := &fakeBoardReader{}
		svc, _, _ := newCompanyWatch(boards)
		watchAll(t, svc, "a", "b")
		boards.errs = map[string]error{"a": unreachable, "b": unreachable}

		_, err := svc.Source().Search(ctx, domain.SearchCriteria{Title: "engineer"})
		assert.ErrorIs(t, err, ports.ErrSourceUnreachable)
	})

	t.Run("every company failing, one reached, is the source failed", func(t *testing.T) {
		t.Parallel()
		boards := &fakeBoardReader{}
		svc, _, _ := newCompanyWatch(boards)
		watchAll(t, svc, "a", "b")
		boards.errs = map[string]error{"a": unreachable, "b": ports.ErrBoardGone}

		_, err := svc.Source().Search(ctx, domain.SearchCriteria{Title: "engineer"})
		require.Error(t, err)
		assert.NotErrorIs(t, err, ports.ErrSourceUnreachable)
	})
}

// Discovery reports a partial answer as an answer that names what failed,
// and keeps every other source's jobs (AC-005).
func TestJobDiscoveryService_Search_CompanyPages(t *testing.T) {
	t.Parallel()
	boards := &fakeBoardReader{
		jobs:  map[string][]domain.Job{"gitlab": boardJobs("gitlab", "AI Engineer", "Account Executive")},
		names: map[string]string{"gitlab": "GitLab", "closed": "Closed Co"},
	}
	companies, _, _ := newCompanyWatch(boards)
	watchAll(t, companies, "gitlab", "closed")
	boards.errs = map[string]error{"closed": ports.ErrBoardGone}

	board := searchableSource("board", []domain.Job{{CanonicalURL: "https://acme.example/1", Title: "Platform Engineer"}}, nil)
	svc := NewJobDiscoveryService(newFakeJobRepository(), []ports.JobSource{board, companies.Source()}, nil, nil, nil)

	result, err := svc.Search(context.Background(), domain.SearchCriteria{Title: "engineer"})
	require.NoError(t, err)

	titles := make([]string, len(result.Jobs))
	for i, job := range result.Jobs {
		titles[i] = job.Title
	}
	assert.ElementsMatch(t, []string{"Platform Engineer", "AI Engineer"}, titles, "the same criteria as every source's (AC-003)")
	assert.Equal(t, domain.SourceOutcome{
		SourceID: "company-pages", SourceName: "Company pages", Status: domain.SourceStatusOK, Found: 1,
		Failing: []string{"Closed Co"},
	}, result.Sources[1])
	assert.Empty(t, result.Unanswered(), "the source answered, for the companies that did")

	job := result.Jobs[1]
	if !strings.Contains(job.Title, "AI") {
		job = result.Jobs[0]
	}
	assert.Equal(t, domain.WorkableAnywhere, job.Workable.Scope, "read like every other source's (AC-004)")
}
