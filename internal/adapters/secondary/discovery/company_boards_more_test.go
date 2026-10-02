//go:build integration

package discovery

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// The fixtures are cut down from the providers' answers on 2026-10-02.
const (
	workableWidgetJSON = `{"name":"Hugging Face","description":"Here at Hugging Face…","jobs":[
		{"title":"Low-level Senior Software Engineer, Xet Storage - EMEA Remote","shortcode":"F4C096B22E","code":"",
		"employment_type":"Full-time","telecommuting":true,"department":"Product",
		"url":"https://apply.workable.com/j/F4C096B22E","shortlink":"https://apply.workable.com/j/F4C096B22E",
		"application_url":"https://apply.workable.com/j/F4C096B22E/apply","published_on":"2026-07-30","created_at":"2026-07-30",
		"country":"France","city":"Paris","state":"Île-de-France",
		"locations":[{"country":"France","countryCode":"FR","city":"Paris","region":"Île-de-France","hidden":true}],
		"description":"<p>At Hugging Face, we're on a journey to democratize <strong>good AI</strong>.</p><p><strong>What we're looking for</strong></p><ul><li>Rust</li></ul>"},
		{"title":"Open-Source Machine Learning Engineer - EMEA Remote","shortcode":"81B46579FE","employment_type":"Full-time",
		"telecommuting":true,"url":"https://apply.workable.com/j/81B46579FE","published_on":"2026-05-29","created_at":"2026-05-29",
		"locations":[{"country":"France","countryCode":"FR","city":"Paris","region":"Île-de-France","hidden":false}],
		"description":"<p>Open source.</p>"},
		{"title":"Office Manager","shortcode":"0FF1CE0000","employment_type":"Part-time","telecommuting":false,
		"published_on":"2026-09-01","locations":[{"country":"United Kingdom","countryCode":"GB","city":"London","region":"England","hidden":false}],
		"description":"<p>In the office.</p>"},
		{"title":"","shortcode":"NOTITLE000","description":""}]}`
	smartRecruitersPostingJSON = `{"id":"%s","name":"%s","company":{"identifier":"Sectigo","name":"Sectigo"},
		"releasedDate":"2026-10-02T12:15:46.155Z","location":{"city":"Paris","region":"IDF","country":"fr","remote":%t,"hybrid":%t,
		"fullLocation":"Paris, IDF, France"},"typeOfEmployment":{"id":"permanent","label":"Full-time"},
		"ref":"https://api.smartrecruiters.com/v1/companies/Sectigo/postings/%s"}`
	smartRecruitersDetailsJSON = `{"id":"3743990015853896","name":"Enterprise Onboarding Engineer",
		"postingUrl":"https://jobs.smartrecruiters.com/Sectigo/3743990015853896-enterprise-onboarding-engineer",
		"jobAd":{"sections":{
		"companyDescription":{"title":"Company Description","text":"<p><strong>Sectigo</strong> is the most innovative provider of certificate lifecycle management.</p>"},
		"jobDescription":{"title":"Job Description","text":"<p>Onboard enterprise customers onto Kubernetes.</p>"},
		"qualifications":{"title":"Qualifications","text":"<ul><li>Bachelor’s degree.</li></ul>"},
		"additionalInformation":{"title":"Additional Information","text":"<p>Sectigo is an Equal Opportunity Employer.</p>"},
		"videos":{"title":"Videos To Watch","urls":["https://www.youtube.com/watch?v=R2zvMKayp8I"]}}}}`
)

// sectigoPostings is a SmartRecruiters company with n postings, every third
// remote and every fifth hybrid.
func sectigoPostings(n int) []string {
	postings := make([]string, n)
	for i := range postings {
		id := strconv.Itoa(3743990015853896 + i)
		name := "Enterprise Onboarding Engineer"
		if i > 0 {
			name = fmt.Sprintf("Role %d", i)
		}
		postings[i] = fmt.Sprintf(smartRecruitersPostingJSON, id, name, i%3 == 0, i%5 == 4, id)
	}
	return postings
}

// moreBoardServer serves Workable and SmartRecruiters as they answered when
// measured: a missing Workable account is a 404, a SmartRecruiters company
// is listed a hundred postings at a time and a missing one is an empty list,
// whose careers page redirects to SmartRecruiters' search.
func moreBoardServer(t *testing.T, postings []string) (BoardEndpoints, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	var asked, described atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		switch r.URL.Path {
		case "/workable/api/v1/widget/accounts/huggingface":
			assert.Equal(t, "true", r.URL.Query().Get("details"))
			_, _ = w.Write([]byte(workableWidgetJSON))
		case "/workable/api/v1/widget/accounts/busy":
			w.WriteHeader(http.StatusTooManyRequests)
		case "/workable/j/F4C096B22E":
			http.Redirect(w, r, "/huggingface/j/F4C096B22E", http.StatusMovedPermanently)
		case "/sr/v1/companies/Sectigo/postings", "/sr/v1/companies/sectigo/postings":
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			assert.Equal(t, 100, limit)
			page := postings[min(offset, len(postings)):min(offset+limit, len(postings))]
			_, _ = fmt.Fprintf(w, `{"offset":%d,"limit":%d,"totalFound":%d,"content":[%s]}`,
				offset, limit, len(postings), joinJSON(page))
		case "/sr/v1/companies/Visa/postings", "/sr/v1/companies/nobody/postings", "/sr/v1/companies/Bosch/postings":
			_, _ = w.Write([]byte(`{"offset":0,"limit":100,"totalFound":0,"content":[]}`))
		case "/sr-pages/Visa":
			_, _ = w.Write([]byte(`<html><head><title>Careers at Visa</title></head></html>`))
		case "/sr-pages/Bosch":
			http.Redirect(w, r, "https://jobs.bosch.com/en", http.StatusFound)
		case "/sr-pages/nobody":
			http.Redirect(w, r, "https://jobs.smartrecruiters.com", http.StatusFound)
		case "/sr/v1/companies/Sectigo/postings/3743990015853896", "/sr/v1/companies/Sectigo/postings/3743990015853897":
			described.Add(1)
			_, _ = w.Write([]byte(smartRecruitersDetailsJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return BoardEndpoints{
		WorkableAPI:          server.URL + "/workable",
		SmartRecruitersAPI:   server.URL + "/sr",
		SmartRecruitersPages: server.URL + "/sr-pages",
	}, &asked, &described
}

func joinJSON(items []string) string {
	joined := ""
	for i, item := range items {
		if i > 0 {
			joined += ","
		}
		joined += item
	}
	return joined
}

func workable(name string) domain.Board {
	return domain.Board{Provider: domain.ProviderWorkable, Name: name}
}

func smartRecruiters(name string) domain.Board {
	return domain.Board{Provider: domain.ProviderSmartRecruiters, Name: name}
}

func TestCompanyBoards_Workable(t *testing.T) {
	t.Parallel()
	endpoints, asked, _ := moreBoardServer(t, nil)
	boards := NewCompanyBoards(endpoints)
	ctx := context.Background()

	contents, err := boards.ReadBoard(ctx, workable("huggingface"))
	require.NoError(t, err)
	assert.False(t, contents.Undescribed, "the widget gives every description in one answer")
	require.Len(t, contents.Jobs, 3, "a posting without a title is dropped")

	job := contents.Jobs[0]
	assert.Equal(t, "https://apply.workable.com/huggingface/j/F4C096B22E/", job.CanonicalURL,
		"the posting's address names the account, which the widget's does not")
	assert.Equal(t, "workable/huggingface/F4C096B22E", job.SourceJobID)
	assert.Equal(t, "https://apply.workable.com/huggingface/", job.CompanyURL)
	assert.Empty(t, job.Locations, "a hidden location is not published, so not read")
	assert.Equal(t, domain.RemoteTypeRemote, job.RemoteType)
	assert.Equal(t, "full-time", job.EmploymentType)
	assert.Contains(t, job.DescriptionMarkdown, "**good AI**")
	assert.Contains(t, job.DescriptionMarkdown, "Rust", "the widget's description holds the requirements")
	assert.Equal(t, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC), job.PostedAt)
	assert.Empty(t, job.CompanyName, "the company is named by the board")

	visible := contents.Jobs[1]
	assert.Equal(t, []string{"Paris, Île-de-France, France"}, visible.Locations)
	office := contents.Jobs[2]
	assert.Equal(t, "", office.RemoteType, "not remote is all Workable says: hybrid and on-site are not told apart")
	assert.Equal(t, "part-time", office.EmploymentType)

	name, err := boards.BoardName(ctx, workable("huggingface"))
	require.NoError(t, err)
	assert.Equal(t, "Hugging Face", name)
	assert.Equal(t, int32(1), asked.Load(), "Workable limits how often it is asked, so its name comes from the read")

	_, err = boards.ReadBoard(ctx, workable("nobody"))
	assert.ErrorIs(t, err, ports.ErrBoardGone)

	_, err = boards.ReadBoard(ctx, workable("busy"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ports.ErrBoardBusy)
	assert.NotErrorIs(t, err, ports.ErrBoardGone, "asked to wait is not gone")
}

func TestCompanyBoards_ResolveAddress(t *testing.T) {
	t.Parallel()
	endpoints, _, _ := moreBoardServer(t, nil)
	boards := NewCompanyBoards(endpoints)
	ctx := context.Background()

	resolved, err := boards.ResolveAddress(ctx, endpoints.WorkableAPI+"/j/F4C096B22E")
	require.NoError(t, err)
	host := strings.TrimSuffix(endpoints.WorkableAPI, "/workable")
	assert.Equal(t, host+"/huggingface/j/F4C096B22E", resolved, "Workable's redirect is a path, read against the address's host")

	_, err = boards.ResolveAddress(ctx, endpoints.WorkableAPI+"/j/0000000000")
	assert.ErrorIs(t, err, ports.ErrBoardGone)
}

func TestCompanyBoards_SmartRecruiters(t *testing.T) {
	t.Parallel()
	endpoints, asked, described := moreBoardServer(t, sectigoPostings(230))
	boards := NewCompanyBoards(endpoints)
	ctx := context.Background()

	contents, err := boards.ReadBoard(ctx, smartRecruiters("Sectigo"))
	require.NoError(t, err)
	assert.True(t, contents.Undescribed)
	require.Len(t, contents.Jobs, 230, "every page of the list")
	assert.Equal(t, int32(3), asked.Load(), "a hundred postings a request")

	job := contents.Jobs[0]
	assert.Equal(t, "https://jobs.smartrecruiters.com/Sectigo/3743990015853896", job.CanonicalURL)
	assert.Equal(t, "smartrecruiters/Sectigo/3743990015853896", job.SourceJobID)
	assert.Equal(t, "https://careers.smartrecruiters.com/Sectigo", job.CompanyURL)
	assert.Equal(t, "Sectigo", job.CompanyName)
	assert.Equal(t, []string{"Paris, IDF, France"}, job.Locations)
	assert.Equal(t, domain.RemoteTypeRemote, job.RemoteType)
	assert.Equal(t, "full-time", job.EmploymentType)
	assert.Empty(t, job.DescriptionMarkdown, "the list gives none")
	assert.Equal(t, domain.RemoteTypeHybrid, contents.Jobs[4].RemoteType)
	assert.Equal(t, "", contents.Jobs[1].RemoteType)

	full, err := boards.DescribeJobs(ctx, smartRecruiters("Sectigo"), contents.Jobs[:3])
	require.NoError(t, err)
	require.Len(t, full, 2, "a posting closed since the list is left out")
	assert.Contains(t, full[0].DescriptionMarkdown, "Company Description")
	assert.Contains(t, full[0].DescriptionMarkdown, "Kubernetes")
	assert.Contains(t, full[0].DescriptionMarkdown, "Equal Opportunity")
	assert.Equal(t, int32(2), described.Load(), "two described, and one asked for and gone")

	_, err = boards.DescribeJobs(ctx, smartRecruiters("Sectigo"), contents.Jobs[:2])
	require.NoError(t, err)
	assert.Equal(t, int32(2), described.Load(), "a posting described before is not asked for again")

	name, err := boards.BoardName(ctx, smartRecruiters("Sectigo"))
	require.NoError(t, err)
	assert.Equal(t, "Sectigo", name, "named by its postings")
}

func TestCompanyBoards_SmartRecruitersWithoutPostings(t *testing.T) {
	t.Parallel()
	endpoints, _, _ := moreBoardServer(t, nil)
	boards := NewCompanyBoards(endpoints)
	ctx := context.Background()

	contents, err := boards.ReadBoard(ctx, smartRecruiters("Visa"))
	require.NoError(t, err)
	assert.Empty(t, contents.Jobs)
	name, err := boards.BoardName(ctx, smartRecruiters("Visa"))
	require.NoError(t, err)
	assert.Equal(t, "Visa", name, "a company with no postings is named by its careers page")

	_, err = boards.ReadBoard(ctx, smartRecruiters("Bosch"))
	require.NoError(t, err, "a careers page sent to the company's own site is still a company")
	name, err = boards.BoardName(ctx, smartRecruiters("Bosch"))
	require.NoError(t, err)
	assert.Empty(t, name)

	_, err = boards.ReadBoard(ctx, smartRecruiters("nobody"))
	assert.ErrorIs(t, err, ports.ErrBoardGone, "SmartRecruiters lists no postings for a company that does not exist")
}

func TestCompanyBoards_DescribeLeavesOtherProvidersAsTheyAre(t *testing.T) {
	t.Parallel()
	jobs := []domain.Job{{Title: "AI Engineer", DescriptionMarkdown: "Already described."}}

	described, err := NewCompanyBoards(BoardEndpoints{}).DescribeJobs(context.Background(), greenhouse("gitlab"), jobs)
	require.NoError(t, err)
	assert.Equal(t, jobs, described)
}
