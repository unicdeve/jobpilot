//go:build integration

package discovery

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

const himalayasPage = `{
	"totalCount": 100632,
	"jobs": [
		{
			"guid": "job-1",
			"title": "Staff Platform Engineer",
			"companyName": "iFIT",
			"description": "<p>Run the <strong>platform</strong>.</p>",
			"excerpt": "Run the platform.",
			"applicationLink": "https://himalayas.app/companies/ifit/jobs/staff-platform-engineer",
			"locationRestrictions": ["United States"],
			"employmentType": "Full Time",
			"minSalary": 155000,
			"maxSalary": 190000,
			"currency": "usd",
			"salaryPeriod": "Yearly",
			"categories": ["Engineering"],
			"pubDate": 1790203312,
			"expiryDate": 1792795312
		},
		{
			"guid": "job-2",
			"title": "Support Lead",
			"companyName": "Widget",
			"description": "",
			"excerpt": "Lead support.",
			"applicationLink": "https://himalayas.app/companies/widget/jobs/support-lead",
			"locationRestrictions": [],
			"employmentType": "Sabbatical",
			"minSalary": 0,
			"maxSalary": 0
		},
		{"guid": "job-3", "title": "No link", "applicationLink": ""}
	]
}`

func TestHimalayasSource_Search(t *testing.T) {
	t.Parallel()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Only the first page has jobs, so the fetch stops early rather
		// than asking for all four.
		if atomic.AddInt32(&calls, 1) == 1 {
			_, _ = w.Write([]byte(himalayasPage))
			return
		}
		_, _ = w.Write([]byte(`{"totalCount":0,"jobs":[]}`))
	}))
	t.Cleanup(server.Close)

	jobs, err := NewHimalayasSource(server.URL).Search(context.Background(), domain.SearchCriteria{})
	require.NoError(t, err)

	require.Len(t, jobs, 2, "a job with no application link is not applyable, so it is not a job")

	first := jobs[0]
	assert.Equal(t, "Staff Platform Engineer", first.Title)
	assert.Equal(t, "himalayas", first.SourceID)
	assert.Equal(t, "job-1", first.SourceJobID)
	assert.Equal(t, []string{"United States"}, first.Locations)
	assert.Equal(t, "remote", first.RemoteType)
	assert.Equal(t, "full-time", first.EmploymentType, `"Full Time" is the vocabulary's "full-time"`)
	assert.Equal(t, 155000.0, first.SalaryMin)
	assert.Equal(t, "USD", first.SalaryCurrency)
	assert.Equal(t, "yearly", first.SalaryPeriod)
	assert.False(t, first.PostedAt.IsZero())
	assert.False(t, first.ExpiresAt.IsZero())
	assert.NotContains(t, first.DescriptionMarkdown, "<strong>")

	second := jobs[1]
	assert.Equal(t, []string{"Worldwide"}, second.Locations,
		"Himalayas writes worldwide as no restriction, which would otherwise read as not stated")
	assert.Equal(t, "Lead support.", second.DescriptionMarkdown,
		"a job with no description falls back to its excerpt rather than being blank")
	assert.Empty(t, second.EmploymentType,
		"a type outside the vocabulary is dropped: one nothing can compare fails every filter silently")
	assert.Zero(t, second.SalaryMin)
}

func TestHimalayasSource_PagesThroughTheBoard(t *testing.T) {
	t.Parallel()

	var offsets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offsets = append(offsets, r.URL.Query().Get("offset"))
		_, _ = fmt.Fprintf(w, `{"totalCount":100,"jobs":[{"guid":"g%s","title":"Engineer","companyName":"Acme","applicationLink":"https://himalayas.app/j/%s"}]}`,
			r.URL.Query().Get("offset"), r.URL.Query().Get("offset"))
	}))
	t.Cleanup(server.Close)

	jobs, err := NewHimalayasSource(server.URL).Search(context.Background(), domain.SearchCriteria{})
	require.NoError(t, err)

	assert.Len(t, jobs, himalayasMaxPages)
	assert.Equal(t, []string{"0", "20", "40", "60"}, offsets,
		"the board holds a hundred thousand jobs, so a fetch takes a bounded recent slice")
}

func TestHimalayasSource_SearchesForTheCriteria(t *testing.T) {
	t.Parallel()

	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path+"?"+r.URL.RawQuery)
		page := r.URL.Query().Get("page")
		if page == "3" {
			_, _ = w.Write([]byte(`{"totalCount":41,"jobs":[]}`))
			return
		}
		// Page 2 repeats page 1's posting, as a board that changed between
		// the two requests does.
		_, _ = fmt.Fprintf(w, `{"totalCount":41,"jobs":[
			{"guid":"a","title":"Backend Engineer","companyName":"Acme","applicationLink":"https://himalayas.app/j/a"},
			{"guid":"p%s","title":"Backend Engineer","companyName":"Acme","applicationLink":"https://himalayas.app/j/p%s"}]}`, page, page)
	}))
	t.Cleanup(server.Close)

	source := NewHimalayasSource(server.URL)
	criteria := domain.SearchCriteria{Title: "backend", Keywords: "  engineer ", WorkFrom: "ng"}
	jobs, err := source.Search(context.Background(), criteria)
	require.NoError(t, err)

	assert.Equal(t, []string{
		"/search?country=NG&page=1&q=backend+engineer",
		"/search?country=NG&page=2&q=backend+engineer",
		"/search?country=NG&page=3&q=backend+engineer",
	}, requests, "the search endpoint pages from one and stops at an empty page")
	assert.Len(t, jobs, 3, "a posting on two pages is one job")

	_, err = source.Search(context.Background(), criteria)
	require.NoError(t, err)
	assert.Len(t, requests, 3, "the same query within the hour is answered from the cache")

	_, err = source.Search(context.Background(), domain.SearchCriteria{Title: "frontend"})
	require.NoError(t, err)
	assert.Equal(t, "/search?page=1&q=frontend", requests[3],
		"another query is its own request, and no country is sent when none is set")
}

func TestHimalayasSource_SearchReadsAtMostFivePages(t *testing.T) {
	t.Parallel()

	var pages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		_, _ = fmt.Fprintf(w, `{"totalCount":500,"jobs":[{"guid":"g%s","title":"Engineer","companyName":"Acme","applicationLink":"https://himalayas.app/j/%s"}]}`, page, page)
	}))
	t.Cleanup(server.Close)

	jobs, err := NewHimalayasSource(server.URL).Search(context.Background(), domain.SearchCriteria{Keywords: "go"})
	require.NoError(t, err)
	assert.Len(t, jobs, himalayasMaxSearchPages)
	assert.Equal(t, []string{"1", "2", "3", "4", "5"}, pages)
}

func TestHimalayasSource_KeepsWhatItAlreadyFetched(t *testing.T) {
	t.Parallel()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			_, _ = w.Write([]byte(himalayasPage))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	jobs, err := NewHimalayasSource(server.URL).Search(context.Background(), domain.SearchCriteria{})

	require.NoError(t, err, "a later page failing must not discard the pages that worked")
	assert.Len(t, jobs, 2)
}

func TestHimalayasSource_FirstPageFailureIsAnError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	_, err := NewHimalayasSource(server.URL).Search(context.Background(), domain.SearchCriteria{})
	assert.ErrorContains(t, err, "503")
}

func TestNormalizeEmploymentType(t *testing.T) {
	t.Parallel()

	for written, want := range map[string]string{
		"Full Time": "full-time", "full_time": "full-time", "FULL-TIME": "full-time",
		"Permanent": "full-time", "Contract": "contract", "Freelance": "contract",
		"Intern": "internship", "Internship": "internship",
		"Sabbatical": "", "": "", "something else": "",
	} {
		assert.Equal(t, want, normalizeEmploymentType(written), "%q", written)
	}
}
