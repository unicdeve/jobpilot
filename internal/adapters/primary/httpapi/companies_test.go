package httpapi

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
)

type fakeWatchedCompanies struct {
	mu        sync.Mutex
	companies []*domain.WatchedCompany
}

func (f *fakeWatchedCompanies) Create(_ context.Context, company *domain.WatchedCompany) (*domain.WatchedCompany, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	company.ID = fmt.Sprintf("company-%d", len(f.companies)+1)
	company.URL = company.Board.URL()
	f.companies = append(f.companies, company)
	return company, nil
}

func (f *fakeWatchedCompanies) find(match func(*domain.WatchedCompany) bool) *domain.WatchedCompany {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, company := range f.companies {
		if match(company) {
			return company
		}
	}
	return nil
}

func (f *fakeWatchedCompanies) GetByID(_ context.Context, id string) (*domain.WatchedCompany, error) {
	if company := f.find(func(c *domain.WatchedCompany) bool { return c.ID == id }); company != nil {
		return company, nil
	}
	return nil, ports.ErrWatchedCompanyNotFound
}

func (f *fakeWatchedCompanies) GetByBoard(_ context.Context, board domain.Board) (*domain.WatchedCompany, error) {
	return f.find(func(c *domain.WatchedCompany) bool { return c.Board.Same(board) }), nil
}

func (f *fakeWatchedCompanies) List(context.Context) ([]*domain.WatchedCompany, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*domain.WatchedCompany{}, f.companies...), nil
}

func (f *fakeWatchedCompanies) Rename(ctx context.Context, id, name string) error {
	company, err := f.GetByID(ctx, id)
	if err == nil {
		company.Name = name
	}
	return err
}

func (f *fakeWatchedCompanies) RecordCheck(ctx context.Context, id string, at time.Time, check domain.BoardCheck, openRoles int) error {
	company, err := f.GetByID(ctx, id)
	if err == nil {
		company.CheckedAt, company.Check, company.OpenRoles = &at, check, openRoles
	}
	return err
}

func (f *fakeWatchedCompanies) Delete(ctx context.Context, id string) error {
	if _, err := f.GetByID(ctx, id); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, company := range f.companies {
		if company.ID == id {
			f.companies = append(f.companies[:i], f.companies[i+1:]...)
		}
	}
	return nil
}

// fakeBoards answers every board with one posting, except those named
// "closed", which have gone, and "down", which do not answer.
type fakeBoards struct{}

func (fakeBoards) ReadBoard(_ context.Context, board domain.Board) (*domain.BoardContents, error) {
	switch board.Name {
	case "closed":
		return nil, ports.ErrBoardGone
	case "down":
		return nil, fmt.Errorf("dial: %w", ports.ErrSourceUnreachable)
	}
	return &domain.BoardContents{Jobs: []domain.Job{{Title: "Engineer"}}, FetchedAt: time.Now()}, nil
}

func (fakeBoards) BoardName(_ context.Context, board domain.Board) (string, error) {
	return strings.ToUpper(board.Name[:1]) + board.Name[1:] + " Inc", nil
}

func companiesServer(t *testing.T) (*Client, *fakeJobRepository) {
	t.Helper()
	jobs := newFakeJobRepository()
	svc := services.NewCompanyWatchService(&fakeWatchedCompanies{}, fakeBoards{}, jobs)
	ts := httptest.NewServer(NewHandler(Services{Companies: svc}))
	t.Cleanup(ts.Close)
	_, portStr, err := net.SplitHostPort(ts.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	return NewClient(port), jobs
}

func TestCompanies_WatchRenameUnwatch(t *testing.T) {
	t.Parallel()
	client, _ := companiesServer(t)
	ctx := context.Background()

	company, created, err := client.WatchCompany(ctx, "https://jobs.lever.co/spotify/2193db3f")
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "Spotify Inc", company.Name)
	assert.Equal(t, "https://jobs.lever.co/spotify", company.URL)
	assert.Equal(t, domain.BoardCheckOK, company.Check)
	assert.Equal(t, 1, company.OpenRoles)

	again, created, err := client.WatchCompany(ctx, "jobs.lever.co/Spotify")
	require.NoError(t, err)
	assert.False(t, created, "200, not 201, for a company already watched")
	assert.Equal(t, company.ID, again.ID)

	renamed, err := client.RenameCompany(ctx, company.ID, "Spotify")
	require.NoError(t, err)
	assert.Equal(t, "Spotify", renamed.Name)

	companies, err := client.ListCompanies(ctx)
	require.NoError(t, err)
	require.Len(t, companies, 1)

	require.NoError(t, client.UnwatchCompany(ctx, company.ID))
	companies, err = client.ListCompanies(ctx)
	require.NoError(t, err)
	assert.Empty(t, companies)
}

func TestCompanies_Statuses(t *testing.T) {
	t.Parallel()
	client, _ := companiesServer(t)
	ctx := context.Background()

	for address, status := range map[string]int{
		"https://www.linkedin.com/company/gitlab": http.StatusBadRequest,
		"https://jobs.ashbyhq.com/closed":         http.StatusUnprocessableEntity,
		"https://jobs.ashbyhq.com/down":           http.StatusServiceUnavailable,
	} {
		resp, err := client.do(ctx, http.MethodPost, "/v1/companies", watchCompanyRequest{Address: address}, nil)
		require.Error(t, err, address)
		assert.Equal(t, status, resp.StatusCode, address)
	}

	resp, err := client.do(ctx, http.MethodPost, "/v1/companies", watchCompanyRequest{}, nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "an address or a job, not neither")

	resp, err = client.do(ctx, http.MethodPut, "/v1/companies/nope", renameCompanyRequest{Name: "x"}, nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	company, _, err := client.WatchCompany(ctx, "jobs.ashbyhq.com/railway")
	require.NoError(t, err)
	resp, err = client.do(ctx, http.MethodPut, "/v1/companies/"+company.ID, renameCompanyRequest{Name: "  "}, nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	resp, err = client.do(ctx, http.MethodDelete, "/v1/companies/nope", nil, nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestCompanies_FromAJob(t *testing.T) {
	t.Parallel()
	client, jobs := companiesServer(t)
	ctx := context.Background()
	job := &domain.Job{CanonicalURL: "https://job-boards.greenhouse.io/gitlab/jobs/1", Title: "AI Engineer"}
	require.NoError(t, jobs.Save(ctx, job))

	var before services.JobCompany
	_, err := client.do(ctx, http.MethodGet, "/v1/jobs/"+job.ID+"/company", nil, &before)
	require.NoError(t, err)
	require.NotNil(t, before.Board)
	assert.Equal(t, "https://job-boards.greenhouse.io/gitlab", before.URL)
	assert.Nil(t, before.Company)

	var company domain.WatchedCompany
	resp, err := client.do(ctx, http.MethodPost, "/v1/companies", watchCompanyRequest{JobID: job.ID}, &company)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, "Gitlab Inc", company.Name)

	var after services.JobCompany
	_, err = client.do(ctx, http.MethodGet, "/v1/jobs/"+job.ID+"/company", nil, &after)
	require.NoError(t, err)
	require.NotNil(t, after.Company)
	assert.Equal(t, company.ID, after.Company.ID)
}

func TestCompanies_Suggestions(t *testing.T) {
	t.Parallel()
	client, _ := companiesServer(t)

	suggestions, err := client.CompanySuggestions(context.Background())
	require.NoError(t, err)
	assert.Len(t, suggestions, len(domain.StarterCompanies()))
	for _, suggestion := range suggestions {
		assert.NotEmpty(t, suggestion.URL)
		assert.NotEmpty(t, suggestion.MeasuredOn)
	}
}
