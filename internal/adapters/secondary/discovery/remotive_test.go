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

const remotiveFixture = `{
	"job-count": 2,
	"jobs": [
		{
			"id": 555,
			"url": "https://remotive.com/remote-jobs/software-dev/senior-go-engineer-555",
			"title": "Senior Go Engineer",
			"company_name": "Acme Remote",
			"tags": ["golang", "backend"],
			"job_type": "full_time",
			"publication_date": "2026-09-07T01:10:43",
			"candidate_required_location": "Worldwide",
			"description": "<p>We use <strong>Go</strong>.</p>"
		},
		{
			"id": 556,
			"url": "https://remotive.com/remote-jobs/design/product-designer-556",
			"title": "Product Designer",
			"company_name": "Widget Remote",
			"tags": ["design"],
			"job_type": "full_time",
			"publication_date": "2026-09-06T12:00:00",
			"candidate_required_location": "USA",
			"description": "<p>Design role.</p>"
		}
	]
}`

func TestRemotiveSource_Search(t *testing.T) {
	t.Parallel()

	t.Run("maps fields and filters by keyword client-side", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(remotiveFixture))
		}))
		defer server.Close()

		jobs, err := NewRemotiveSource(server.URL).Search(context.Background(), domain.SearchCriteria{Keywords: "Go"})
		require.NoError(t, err)
		require.Len(t, jobs, 1)

		job := jobs[0]
		assert.Equal(t, "remotive", job.SourceID)
		assert.Equal(t, "555", job.SourceJobID)
		assert.Equal(t, "Senior Go Engineer", job.Title)
		assert.Equal(t, "Acme Remote", job.CompanyName)
		assert.Equal(t, "https://remotive.com/remote-jobs/software-dev/senior-go-engineer-555", job.CanonicalURL)
		assert.Equal(t, "remote", job.RemoteType)
		assert.Equal(t, []string{"Worldwide"}, job.Locations)
		assert.Contains(t, job.DescriptionMarkdown, "Go")
		assert.False(t, job.PostedAt.IsZero())
		assert.NotEmpty(t, job.RawSourceReference)
	})

	t.Run("repeated searches within the cache TTL don't refetch", func(t *testing.T) {
		t.Parallel()
		var requests int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(remotiveFixture))
		}))
		defer server.Close()

		source := NewRemotiveSource(server.URL)
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

		_, err := NewRemotiveSource(server.URL).Search(context.Background(), domain.SearchCriteria{})
		assert.ErrorContains(t, err, "non-OK status")
	})
}

func TestRemotiveSource_Capabilities(t *testing.T) {
	t.Parallel()
	caps := NewRemotiveSource(DefaultRemotiveBaseURL).Capabilities()
	assert.True(t, caps.CanSearch)
	assert.False(t, caps.CanGetJob)
}

func TestRemotiveSource_GetJob_Unsupported(t *testing.T) {
	t.Parallel()
	_, err := NewRemotiveSource(DefaultRemotiveBaseURL).GetJob(context.Background(), "https://example.com/job")
	assert.ErrorContains(t, err, "does not support fetching a job by URL")
}
