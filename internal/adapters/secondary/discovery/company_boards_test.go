//go:build integration

package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
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

// The fixtures are cut down from the providers' answers on 2026-09-30.
const (
	greenhouseJobsJSON = `{"jobs":[{"absolute_url":"https://job-boards.greenhouse.io/gitlab/jobs/8556658002",
		"location":{"name":"Remote, Canada; Remote, United Kingdom; Remote, United States"},"id":8556658002,
		"updated_at":"2026-09-29T10:38:50-04:00","title":"AI Engineer","company_name":"GitLab",
		"first_published":"2026-05-22T09:16:29-04:00",
		"content":"&lt;p&gt;GitLab is the &lt;strong&gt;intelligent&lt;/strong&gt; platform.&lt;/p&gt;"},
		{"absolute_url":"https://job-boards.greenhouse.io/gitlab/jobs/2","location":{"name":"Denver, CO - Hybrid"},
		"id":2,"title":"Sales","company_name":"GitLab","content":""},
		{"absolute_url":"","id":3,"title":"No address"}]}`
	greenhouseBoardJSON = `{"name":"GitLab","content":""}`
	leverJobsJSON       = `[{"id":"2193db3f","text":"Android Engineer - Experience","hostedUrl":"https://jobs.lever.co/spotify/2193db3f",
		"description":"<div><p>We design Spotify’s consumer experience.</p></div>",
		"lists":[{"text":"What You'll Do","content":"<li>Develop Kotlin code</li>"}],
		"additional":"<div>Spotify is an equal opportunity employer.</div>",
		"categories":{"commitment":"Permanent","location":"London","allLocations":["London","Stockholm"]},
		"country":"GB","workplaceType":"hybrid","createdAt":1782214185805,
		"salaryRange":{"currency":"gbp","interval":"per-year-salary","min":70000,"max":90000}},
		{"id":"x","text":"Unspecified","hostedUrl":"https://jobs.lever.co/spotify/x","categories":{"location":"Remote"},
		"workplaceType":"unspecified"}]`
	leverPageHTML = `<html><head><title>Spotify</title></head><body></body></html>`
	ashbyJobsJSON = `{"jobs":[{"id":"1e8c7f3d","title":"Strategic Solutions Engineer","employmentType":"FullTime",
		"location":"Global","secondaryLocations":[{"location":"EMEA"}],"publishedAt":"2026-09-21T02:50:22.578+00:00",
		"isListed":true,"isRemote":true,"workplaceType":"Remote",
		"address":{"postalAddress":{"addressCountry":"United States","addressLocality":"San Francisco"}},
		"jobUrl":"https://jobs.ashbyhq.com/railway/1e8c7f3d","descriptionHtml":"<p>Help customers.</p>",
		"compensation":{"summaryComponents":[{"compensationType":"EquityPercentage","interval":"NONE"},
			{"compensationType":"Salary","interval":"1 YEAR","currencyCode":"USD","minValue":200000,"maxValue":250000}]}},
		{"id":"hidden","title":"Unlisted","jobUrl":"https://jobs.ashbyhq.com/railway/hidden","isListed":false},
		{"id":"old","title":"No workplace","jobUrl":"https://jobs.ashbyhq.com/railway/old","location":"Remote, AMER",
		"isRemote":true}]}`
	ashbyPageHTML = `<html><head><title>Railway Jobs</title></head><body><script>window.__appData = {"organization":{"organizationId":"2f86","name":"Railway & Co","publicWebsite":"https://railway.com/"}};</script></body></html>`
)

// boardServer serves every provider's endpoints from one server, counting
// the requests it answers.
func boardServer(t *testing.T) (BoardEndpoints, *atomic.Int32) {
	t.Helper()
	var asked atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		switch r.URL.Path {
		case "/greenhouse/v1/boards/gitlab/jobs":
			assert.Equal(t, "true", r.URL.Query().Get("content"))
			_, _ = w.Write([]byte(greenhouseJobsJSON))
		case "/greenhouse/v1/boards/gitlab":
			_, _ = w.Write([]byte(greenhouseBoardJSON))
		case "/lever/v0/postings/spotify":
			_, _ = w.Write([]byte(leverJobsJSON))
		case "/lever-pages/spotify":
			_, _ = w.Write([]byte(leverPageHTML))
		case "/ashby/posting-api/job-board/railway":
			assert.Equal(t, "true", r.URL.Query().Get("includeCompensation"))
			_, _ = w.Write([]byte(ashbyJobsJSON))
		case "/ashby-pages/railway":
			_, _ = w.Write([]byte(ashbyPageHTML))
		case "/ashby-pages/nameless":
			_, _ = w.Write([]byte(`<html><title>Jobs</title></html>`))
		case "/greenhouse/v1/boards/broken/jobs":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return BoardEndpoints{
		GreenhouseAPI:   server.URL + "/greenhouse",
		GreenhouseEUAPI: server.URL + "/greenhouse-eu",
		LeverAPI:        server.URL + "/lever",
		LeverEUAPI:      server.URL + "/lever-eu",
		LeverPages:      server.URL + "/lever-pages",
		LeverEUPages:    server.URL + "/lever-eu-pages",
		AshbyAPI:        server.URL + "/ashby",
		AshbyPages:      server.URL + "/ashby-pages",
	}, &asked
}

func greenhouse(name string) domain.Board {
	return domain.Board{Provider: domain.ProviderGreenhouse, Name: name}
}

func TestCompanyBoards_Greenhouse(t *testing.T) {
	t.Parallel()
	endpoints, _ := boardServer(t)

	contents, err := NewCompanyBoards(endpoints).ReadBoard(context.Background(), greenhouse("gitlab"))
	require.NoError(t, err)
	require.Len(t, contents.Jobs, 2, "a posting without an address is dropped")

	job := contents.Jobs[0]
	assert.Equal(t, "https://job-boards.greenhouse.io/gitlab/jobs/8556658002", job.CanonicalURL)
	assert.Equal(t, "company-pages", job.SourceID)
	assert.Equal(t, "greenhouse/gitlab/8556658002", job.SourceJobID)
	assert.Equal(t, "GitLab", job.CompanyName)
	assert.Equal(t, "https://job-boards.greenhouse.io/gitlab", job.CompanyURL)
	assert.Equal(t, []string{"Remote, Canada", "Remote, United Kingdom", "Remote, United States"}, job.Locations)
	assert.Equal(t, domain.RemoteTypeRemote, job.RemoteType)
	assert.Contains(t, job.DescriptionMarkdown, "**intelligent**", "the content is HTML escaped once more")
	assert.Equal(t, time.Date(2026, 5, 22, 13, 16, 29, 0, time.UTC), job.PostedAt)

	assert.Equal(t, domain.RemoteTypeHybrid, contents.Jobs[1].RemoteType)
	assert.WithinDuration(t, time.Now(), contents.FetchedAt, time.Minute)
}

func TestCompanyBoards_Lever(t *testing.T) {
	t.Parallel()
	endpoints, _ := boardServer(t)

	contents, err := NewCompanyBoards(endpoints).ReadBoard(context.Background(),
		domain.Board{Provider: domain.ProviderLever, Name: "spotify"})
	require.NoError(t, err)
	require.Len(t, contents.Jobs, 2)

	job := contents.Jobs[0]
	assert.Equal(t, "https://jobs.lever.co/spotify/2193db3f", job.CanonicalURL)
	assert.Equal(t, []string{"London", "Stockholm"}, job.Locations, "the country code names the office, not where")
	assert.Equal(t, domain.RemoteTypeHybrid, job.RemoteType)
	assert.Equal(t, "full-time", job.EmploymentType)
	assert.Contains(t, job.DescriptionMarkdown, "consumer experience")
	assert.Contains(t, job.DescriptionMarkdown, "What You'll Do")
	assert.Contains(t, job.DescriptionMarkdown, "Develop Kotlin code")
	assert.Contains(t, job.DescriptionMarkdown, "equal opportunity")
	assert.Equal(t, 70000.0, job.SalaryMin)
	assert.Equal(t, 90000.0, job.SalaryMax)
	assert.Equal(t, "GBP", job.SalaryCurrency)
	assert.Equal(t, "year", job.SalaryPeriod)
	assert.Empty(t, job.CompanyName, "Lever's posting does not name the company")

	unspecified := contents.Jobs[1]
	assert.Equal(t, "", unspecified.RemoteType)
	assert.Equal(t, []string{"Remote"}, unspecified.Locations, "without allLocations, the location")
}

func TestCompanyBoards_Ashby(t *testing.T) {
	t.Parallel()
	endpoints, _ := boardServer(t)

	contents, err := NewCompanyBoards(endpoints).ReadBoard(context.Background(),
		domain.Board{Provider: domain.ProviderAshby, Name: "railway"})
	require.NoError(t, err)
	require.Len(t, contents.Jobs, 2, "an unlisted posting is not the company's to show")

	job := contents.Jobs[0]
	assert.Equal(t, "https://jobs.ashbyhq.com/railway/1e8c7f3d", job.CanonicalURL)
	assert.Equal(t, []string{"Global", "EMEA"}, job.Locations, "the address is the office, not where (ADR-0024 §6)")
	assert.Equal(t, domain.RemoteTypeRemote, job.RemoteType)
	assert.Equal(t, "full-time", job.EmploymentType)
	assert.Equal(t, 200000.0, job.SalaryMin)
	assert.Equal(t, 250000.0, job.SalaryMax)
	assert.Equal(t, "USD", job.SalaryCurrency)
	assert.Equal(t, "year", job.SalaryPeriod)
	assert.Equal(t, time.Date(2026, 9, 21, 2, 50, 22, 578000000, time.UTC), job.PostedAt)

	assert.Equal(t, domain.RemoteTypeRemote, contents.Jobs[1].RemoteType, "isRemote stands in for a missing workplace type")

	domain.ReadSignals(&job)
	assert.Equal(t, domain.WorkableAnywhere, job.Workable.Scope)
	older := contents.Jobs[1]
	domain.ReadSignals(&older)
	assert.Equal(t, []string{"Americas"}, older.Workable.Places, "AMER is the Americas")
}

func TestCompanyBoards_BoardName(t *testing.T) {
	t.Parallel()
	endpoints, _ := boardServer(t)
	boards := NewCompanyBoards(endpoints)
	ctx := context.Background()

	for board, want := range map[domain.Board]string{
		greenhouse("gitlab"):                               "GitLab",
		{Provider: domain.ProviderLever, Name: "spotify"}:  "Spotify",
		{Provider: domain.ProviderAshby, Name: "railway"}:  "Railway & Co",
		{Provider: domain.ProviderAshby, Name: "nameless"}: "",
	} {
		name, err := boards.BoardName(ctx, board)
		require.NoError(t, err, board.Name)
		assert.Equal(t, want, name, board.Name)
	}

	_, err := boards.BoardName(ctx, domain.Board{Provider: domain.ProviderLever, Name: "kraken"})
	assert.ErrorIs(t, err, ports.ErrBoardGone, "Lever's page says 404 where its API says []")
}

func TestCompanyBoards_Failures(t *testing.T) {
	t.Parallel()
	endpoints, _ := boardServer(t)
	boards := NewCompanyBoards(endpoints)
	ctx := context.Background()

	_, err := boards.ReadBoard(ctx, greenhouse("closed"))
	assert.ErrorIs(t, err, ports.ErrBoardGone)

	_, err = boards.ReadBoard(ctx, greenhouse("broken"))
	require.Error(t, err)
	assert.NotErrorIs(t, err, ports.ErrBoardGone)
	assert.NotErrorIs(t, err, ports.ErrSourceUnreachable)

	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	_, err = NewCompanyBoards(BoardEndpoints{GreenhouseAPI: server.URL}).ReadBoard(ctx, greenhouse("gitlab"))
	assert.ErrorIs(t, err, ports.ErrSourceUnreachable)
}

func TestCompanyBoards_EUBoardsAreReadFromEUServers(t *testing.T) {
	t.Parallel()
	var paths []string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch {
		case strings.HasPrefix(r.URL.Path, "/gh-eu/"):
			_, _ = w.Write([]byte(`{"jobs":[]}`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(server.Close)
	boards := NewCompanyBoards(BoardEndpoints{GreenhouseEUAPI: server.URL + "/gh-eu", LeverEUAPI: server.URL + "/lever-eu"})

	_, err := boards.ReadBoard(context.Background(), domain.Board{Provider: domain.ProviderGreenhouse, Region: domain.RegionEU, Name: "n26"})
	require.NoError(t, err)
	_, err = boards.ReadBoard(context.Background(), domain.Board{Provider: domain.ProviderLever, Region: domain.RegionEU, Name: "acme"})
	require.NoError(t, err)
	assert.Equal(t, []string{"/gh-eu/v1/boards/n26/jobs", "/lever-eu/v0/postings/acme"}, paths)
}

func TestCompanyBoards_KeepsEachAnswerAnHour(t *testing.T) {
	t.Parallel()
	endpoints, asked := boardServer(t)
	boards := NewCompanyBoards(endpoints)
	ctx := context.Background()

	first, err := boards.ReadBoard(ctx, greenhouse("gitlab"))
	require.NoError(t, err)
	second, err := boards.ReadBoard(ctx, greenhouse("GitLab"))
	require.NoError(t, err)

	assert.Equal(t, int32(1), asked.Load(), "a board's name is not case sensitive, and its answer is kept")
	assert.Equal(t, first.FetchedAt, second.FetchedAt, "a kept answer does not claim a new check")

	_, err = boards.ReadBoard(ctx, greenhouse("broken"))
	require.Error(t, err)
	_, err = boards.ReadBoard(ctx, greenhouse("broken"))
	require.Error(t, err)
	assert.Equal(t, int32(3), asked.Load(), "a failure is not kept")
}

func TestCompanyBoards_ConcurrentReadsShareOneRequest(t *testing.T) {
	t.Parallel()
	var asked atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		<-release
		_, _ = w.Write([]byte(greenhouseJobsJSON))
	}))
	t.Cleanup(server.Close)
	boards := NewCompanyBoards(BoardEndpoints{GreenhouseAPI: server.URL})

	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			_, err := boards.ReadBoard(context.Background(), greenhouse("gitlab"))
			assert.NoError(t, err)
		})
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	assert.Equal(t, int32(1), asked.Load())
}
