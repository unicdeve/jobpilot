//go:build integration

package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

const jobicyLocationsJSON = `{"locations":[
	{"geoName":"Anywhere","geoSlug":"anywhere"},{"geoName":"APAC","geoSlug":"apac"},
	{"geoName":"EMEA","geoSlug":"emea"},{"geoName":"Europe","geoSlug":"europe"},
	{"geoName":"Germany","geoSlug":"germany"},{"geoName":"USA","geoSlug":"usa"}]}`

func jobicyServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var asked []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("get") == "locations" {
			_, _ = w.Write([]byte(jobicyLocationsJSON))
			return
		}
		mu.Lock()
		asked = append(asked, query.Get("geo"))
		mu.Unlock()
		switch query.Get("geo") {
		case "anywhere":
			_, _ = w.Write([]byte(`{"jobs":[{"id":1,"url":"https://jobicy.com/jobs/1-go","jobTitle":"Go Engineer","companyName":"Acme",
				"jobIndustry":["Engineering"],"jobType":["Full-Time"],"jobGeo":"Anywhere","jobDescription":"<p>Build it.</p>",
				"pubDate":"2026-09-30T08:18:51+00:00","salaryMin":148000,"salaryMax":216000,"salaryCurrency":"usd","salaryPeriod":"Yearly"}]}`))
		default:
			_, _ = w.Write([]byte(`{"jobs":[
				{"id":1,"url":"https://jobicy.com/jobs/1-go","jobTitle":"Go Engineer","companyName":"Acme","jobGeo":"Anywhere"},
				{"id":2,"url":"https://jobicy.com/jobs/2-eu","jobTitle":"Account Executive","companyName":"Nash","jobGeo":"Europe,  UK","jobType":["Contract"]}]}`))
		}
	}))
	t.Cleanup(server.Close)
	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := append([]string(nil), asked...)
		sort.Strings(out)
		return out
	}
}

func TestJobicySource_AsksTheFeedsForTheCountry(t *testing.T) {
	t.Parallel()

	server, asked := jobicyServer(t)
	source := NewJobicySource(server.URL)

	jobs, err := source.Search(context.Background(), domain.SearchCriteria{Title: "engineer", WorkFrom: "NG"})
	require.NoError(t, err)
	assert.Equal(t, []string{"anywhere", "emea"}, asked(), "Nigeria is in EMEA, and in none of Jobicy's other locations")
	require.Len(t, jobs, 2, "a job in two feeds is one job")

	first := jobs[0]
	assert.Equal(t, "https://jobicy.com/jobs/1-go", first.CanonicalURL, "the job keeps its Jobicy URL, which the terms require")
	assert.Equal(t, "jobicy", first.SourceID)
	assert.Equal(t, "1", first.SourceJobID)
	assert.Equal(t, []string{"Anywhere"}, first.Locations)
	assert.Equal(t, "full-time", first.EmploymentType)
	assert.Equal(t, 148000.0, first.SalaryMin)
	assert.Equal(t, "USD", first.SalaryCurrency)
	assert.Equal(t, "yearly", first.SalaryPeriod)
	assert.False(t, first.PostedAt.IsZero())
	assert.Equal(t, []string{"Europe", "UK"}, jobs[1].Locations)

	_, err = source.Search(context.Background(), domain.SearchCriteria{Keywords: "sales", WorkFrom: "NG"})
	require.NoError(t, err)
	assert.Len(t, asked(), 2, "a feed is asked at most once an hour, whichever search asks")

	_, err = source.Search(context.Background(), domain.SearchCriteria{Title: "engineer", WorkFrom: "DE"})
	require.NoError(t, err)
	assert.Equal(t, []string{"anywhere", "emea", "europe", "germany"}, asked(),
		"Germany adds its own feeds; the two already read are not read again")
}

func TestJobicySource_WithoutACountryReadsEverything(t *testing.T) {
	t.Parallel()

	server, asked := jobicyServer(t)
	_, err := NewJobicySource(server.URL).Search(context.Background(), domain.SearchCriteria{Title: "engineer"})
	require.NoError(t, err)
	assert.Equal(t, []string{""}, asked(), "no geo is every location")
}

func TestJobicySource_Failure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(server.Close)

	_, err := NewJobicySource(server.URL).Search(context.Background(), domain.SearchCriteria{Title: "engineer"})
	assert.ErrorContains(t, err, "429")
}
