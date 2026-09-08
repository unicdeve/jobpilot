//go:build integration

package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

const arbeitnowFixture = `{
	"data": [
		{
			"slug": "senior-go-engineer-berlin-1",
			"company_name": "Acme GmbH",
			"title": "Senior Go Engineer",
			"description": "<p>We use <strong>Go</strong> and SQLite.</p>",
			"remote": true,
			"url": "https://www.arbeitnow.com/jobs/companies/acme/senior-go-engineer-berlin-1",
			"tags": ["Go", "SQLite"],
			"job_types": ["Full-time"],
			"location": "Berlin; Remote",
			"created_at": 1700000000
		},
		{
			"slug": "frontend-dev-2",
			"company_name": "Widget Co",
			"title": "Frontend Developer",
			"description": "<p>React role.</p>",
			"remote": false,
			"url": "https://www.arbeitnow.com/jobs/companies/widget/frontend-dev-2",
			"tags": ["React"],
			"job_types": [],
			"location": "Lagos",
			"created_at": 1700001000
		}
	],
	"links": {"first": "", "last": null, "prev": null, "next": null},
	"meta": {"current_page": 1}
}`

func TestArbeitnowSource_Search(t *testing.T) {
	t.Parallel()

	t.Run("maps fields and filters by keyword client-side", func(t *testing.T) {
		t.Parallel()
		var requests int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(arbeitnowFixture))
		}))
		defer server.Close()

		source := NewArbeitnowSource(server.URL)

		jobs, err := source.Search(context.Background(), domain.SearchCriteria{Keywords: "Go"})
		require.NoError(t, err)
		require.Len(t, jobs, 1)

		job := jobs[0]
		assert.Equal(t, "arbeitnow", job.SourceID)
		assert.Equal(t, "senior-go-engineer-berlin-1", job.SourceJobID)
		assert.Equal(t, "Senior Go Engineer", job.Title)
		assert.Equal(t, "Acme GmbH", job.CompanyName)
		assert.Equal(t, "https://www.arbeitnow.com/jobs/companies/acme/senior-go-engineer-berlin-1", job.CanonicalURL)
		assert.Equal(t, "remote", job.RemoteType)
		assert.Equal(t, []string{"Berlin", "Remote"}, job.Locations)
		assert.Contains(t, job.DescriptionMarkdown, "Go")
		assert.Equal(t, []string{"Go", "SQLite"}, job.Skills)
		assert.NotEmpty(t, job.RawSourceReference)

		assert.Equal(t, 1, requests)
	})

	t.Run("empty keywords returns everything", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(arbeitnowFixture))
		}))
		defer server.Close()

		jobs, err := NewArbeitnowSource(server.URL).Search(context.Background(), domain.SearchCriteria{})
		require.NoError(t, err)
		assert.Len(t, jobs, 2)
	})

	t.Run("repeated searches within the cache TTL don't refetch", func(t *testing.T) {
		t.Parallel()
		var requests int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(arbeitnowFixture))
		}))
		defer server.Close()

		source := NewArbeitnowSource(server.URL)
		_, err := source.Search(context.Background(), domain.SearchCriteria{})
		require.NoError(t, err)
		_, err = source.Search(context.Background(), domain.SearchCriteria{})
		require.NoError(t, err)

		assert.Equal(t, 1, requests, "second search should be served from cache")
	})

	t.Run("non-200 upstream response is an error", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		_, err := NewArbeitnowSource(server.URL).Search(context.Background(), domain.SearchCriteria{})
		assert.ErrorContains(t, err, "non-OK status")
	})
}

func TestArbeitnowSource_Capabilities(t *testing.T) {
	t.Parallel()
	caps := NewArbeitnowSource(DefaultArbeitnowBaseURL).Capabilities()
	assert.True(t, caps.CanSearch)
	assert.False(t, caps.CanGetJob)
}

func TestArbeitnowSource_GetJob_Unsupported(t *testing.T) {
	t.Parallel()
	_, err := NewArbeitnowSource(DefaultArbeitnowBaseURL).GetJob(context.Background(), "https://example.com/job")
	assert.ErrorContains(t, err, "does not support fetching a job by URL")
}
