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

// remoteOKFixture leads with the feed's legal notice, exactly as the real
// one does.
const remoteOKFixture = `[
	{"legal": "API Terms of Service: Please link back to Remote OK", "last_updated": 1790208007},
	{
		"id": "1137421",
		"slug": "remote-senior-go-engineer",
		"position": "Senior Go Engineer ",
		"company": "Acme ",
		"description": "<p>We use <strong>Go</strong> and SQLite every day.</p>",
		"location": "Germany",
		"tags": ["golang", "backend"],
		"url": "https://remoteOK.com/remote-jobs/remote-senior-go-engineer-1137421",
		"date": "2026-09-23T00:00:10+00:00",
		"salary_min": 120000,
		"salary_max": 160000
	},
	{
		"id": "1137422",
		"position": "Product Designer",
		"company": "Widget",
		"description": "<p>Design role.</p>",
		"location": "",
		"url": "https://remoteOK.com/remote-jobs/product-designer-1137422",
		"date": "2026-09-22T00:00:10+00:00",
		"salary_min": 0,
		"salary_max": 0
	},
	{"id": "9", "position": "", "url": "https://remoteOK.com/x"}
]`

func remoteOKServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NotEmpty(t, r.Header.Get("User-Agent"), "RemoteOK rejects requests without one")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRemoteOKSource_Search(t *testing.T) {
	t.Parallel()

	server := remoteOKServer(t, remoteOKFixture)
	source := NewRemoteOKSource(server.URL)

	jobs, err := source.Search(context.Background(), domain.SearchCriteria{})
	require.NoError(t, err)

	// The legal notice and the entry with no position are not jobs.
	require.Len(t, jobs, 2)

	first := jobs[0]
	assert.Equal(t, "Senior Go Engineer", first.Title, "the feed's trailing spaces are trimmed")
	assert.Equal(t, "Acme", first.CompanyName)
	assert.Equal(t, "remoteok", first.SourceID)
	assert.Equal(t, "1137421", first.SourceJobID)
	assert.Equal(t, []string{"Germany"}, first.Locations)
	assert.Equal(t, "remote", first.RemoteType, "the whole board is remote")
	assert.Equal(t, []string{"golang", "backend"}, first.Skills)
	assert.Contains(t, first.DescriptionMarkdown, "Go")
	assert.NotContains(t, first.DescriptionMarkdown, "<strong>", "HTML should not reach a prompt")
	assert.Equal(t, 2026, first.PostedAt.Year())
	assert.Equal(t, 120000.0, first.SalaryMin)
	assert.Equal(t, "USD", first.SalaryCurrency)

	second := jobs[1]
	assert.Zero(t, second.SalaryMin)
	assert.Empty(t, second.SalaryCurrency,
		"an unstated salary stays unstated rather than becoming a job that pays nothing")
	assert.Empty(t, second.Locations)
}

func TestRemoteOKSource_CachesBetweenSearches(t *testing.T) {
	t.Parallel()

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(remoteOKFixture))
	}))
	t.Cleanup(server.Close)

	source := NewRemoteOKSource(server.URL)
	ctx := context.Background()
	_, err := source.Search(ctx, domain.SearchCriteria{})
	require.NoError(t, err)
	_, err = source.Search(ctx, domain.SearchCriteria{})
	require.NoError(t, err)

	assert.Equal(t, 1, calls, "a second search inside the TTL must not poll the feed again")
}

func TestRemoteOKSource_Errors(t *testing.T) {
	t.Parallel()

	t.Run("a non-OK status is reported", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		t.Cleanup(server.Close)

		_, err := NewRemoteOKSource(server.URL).Search(context.Background(), domain.SearchCriteria{})
		assert.ErrorContains(t, err, "429")
	})

	t.Run("GetJob is unsupported", func(t *testing.T) {
		t.Parallel()
		_, err := NewRemoteOKSource("http://unused").GetJob(context.Background(), "https://x")
		assert.ErrorContains(t, err, "cannot fetch a job by URL")
		assert.False(t, NewRemoteOKSource("http://unused").Capabilities().CanGetJob)
	})
}
