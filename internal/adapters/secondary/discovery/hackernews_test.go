//go:build integration

package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// hnStories includes the two sibling threads posted the same day, because
// picking the wrong one would fill the user's history with people looking
// for work rather than jobs.
const hnStories = `{"hits": [
	{"objectID": "1", "title": "Ask HN: Who wants to be hired? (September 2026)", "created_at": "2026-09-01T15:00:00Z"},
	{"objectID": "2", "title": "Ask HN: Freelancer? Seeking freelancer? (September 2026)", "created_at": "2026-09-01T15:00:00Z"},
	{"objectID": "3", "title": "Ask HN: Who is hiring? (September 2026)", "created_at": "2026-09-01T15:00:00Z"}
]}`

const hnThread = `{
	"id": 3,
	"title": "Ask HN: Who is hiring? (September 2026)",
	"children": [
		{"id": 11, "created_at": "2026-09-01T16:00:00Z",
		 "text": "Modash.io | Senior Product Engineer | Remote (Europe) | Full-time | &euro;75k&ndash;110k | https://modash.io<p>We help brands find creators.</p>"},
		{"id": 12, "created_at": "2026-09-01T17:00:00Z",
		 "text": "Open Education / Neon | Platform &amp; DevOps Engineer | Utrecht, The Netherlands | HYBRID | DUTCH REQUIRED | https://neon.example/jobs"},
		{"id": 13, "created_at": "2026-09-01T18:00:00Z",
		 "text": "Just a comment with no structure at all, wishing everyone luck."},
		{"id": 14, "created_at": "2026-09-01T19:00:00Z",
		 "text": "NoLink Corp | Backend Engineer | Berlin | Full-time"},
		{"id": 15, "created_at": "2026-09-01T20:00:00Z", "text": ""}
	]
}`

func hnServer(t *testing.T, stories, thread string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "search_by_date"):
			_, _ = w.Write([]byte(stories))
		case strings.Contains(r.URL.Path, "/items/"):
			_, _ = w.Write([]byte(thread))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestHackerNewsSource_Search(t *testing.T) {
	t.Parallel()

	server := hnServer(t, hnStories, hnThread)
	jobs, err := NewHackerNewsSource(server.URL).Search(context.Background(), domain.SearchCriteria{})
	require.NoError(t, err)

	// Only the two postings that follow the convention and carry a link.
	require.Len(t, jobs, 2)

	first := jobs[0]
	assert.Equal(t, "Modash.io", first.CompanyName)
	assert.Equal(t, "Senior Product Engineer", first.Title)
	assert.Equal(t, "https://modash.io", first.CanonicalURL)
	assert.Equal(t, "hackernews", first.SourceID)
	assert.Equal(t, "11", first.SourceJobID)
	assert.Equal(t, "remote", first.RemoteType)
	assert.Equal(t, "full-time", first.EmploymentType)
	assert.Equal(t, []string{"Remote (Europe)"}, first.Locations,
		`"EUR" must not match inside "Europe" and turn the location into a salary`)
	assert.Contains(t, first.RawSourceReference, "news.ycombinator.com/item?id=11",
		"a reader can always get back to the thread")

	second := jobs[1]
	assert.Equal(t, "Platform & DevOps Engineer", second.Title,
		"HTML entities survive conversion and must be decoded")
	assert.Equal(t, "hybrid", second.RemoteType)
	assert.Equal(t, []string{"Utrecht, The Netherlands"}, second.Locations,
		`"HYBRID" is a work arrangement and "DUTCH REQUIRED" is a note; neither is a place`)
}

func TestHackerNewsSource_SkipsWhatItCannotRead(t *testing.T) {
	t.Parallel()

	server := hnServer(t, hnStories, hnThread)
	jobs, err := NewHackerNewsSource(server.URL).Search(context.Background(), domain.SearchCriteria{})
	require.NoError(t, err)

	titles := make([]string, 0, len(jobs))
	for _, job := range jobs {
		titles = append(titles, job.Title)
	}
	assert.NotContains(t, titles, "Backend Engineer",
		"a posting with no link cannot be applied to, so it is not stored as a job")
	for _, job := range jobs {
		assert.NotEmpty(t, job.CanonicalURL)
		assert.NotEmpty(t, job.CompanyName)
	}
}

func TestHackerNewsSource_PicksTheHiringThread(t *testing.T) {
	t.Parallel()

	t.Run("ignores the two sibling threads", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isHiringThreadTitle("Ask HN: Who is hiring? (September 2026)"))
		assert.False(t, isHiringThreadTitle("Ask HN: Who wants to be hired? (September 2026)"))
		assert.False(t, isHiringThreadTitle("Ask HN: Freelancer? Seeking freelancer? (September 2026)"))
		assert.False(t, isHiringThreadTitle("Ask HN: Anything else?"))
	})

	t.Run("says so when no thread is found", func(t *testing.T) {
		t.Parallel()
		server := hnServer(t, `{"hits": []}`, hnThread)
		_, err := NewHackerNewsSource(server.URL).Search(context.Background(), domain.SearchCriteria{})
		assert.ErrorContains(t, err, "no 'Who is hiring?' thread found")
	})
}

func TestHackerNewsSource_Capabilities(t *testing.T) {
	t.Parallel()

	source := NewHackerNewsSource("http://unused")
	assert.True(t, source.Capabilities().CanSearch)
	assert.False(t, source.Capabilities().CanGetJob)
	assert.False(t, source.Capabilities().FiltersServerSide)

	_, err := source.GetJob(context.Background(), "https://x")
	assert.ErrorContains(t, err, "cannot fetch a job by URL")
}

func TestLocationsFromFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		fields []string
		want   []string
	}{
		{"a plain place", []string{"Acme", "Engineer", "Berlin, Germany"}, []string{"Berlin, Germany"}},
		{"remote with a region", []string{"Acme", "Engineer", "Remote (Europe)"}, []string{"Remote (Europe)"}},
		{"a bare arrangement is not a place", []string{"Acme", "Engineer", "HYBRID", "Utrecht"}, []string{"Utrecht"}},
		{"a shouted note is not a place", []string{"Acme", "Engineer", "DUTCH REQUIRED"}, nil},
		{"a salary is not a place", []string{"Acme", "Engineer", "$150k - $210k"}, nil},
		{"an employment type is not a place", []string{"Acme", "Engineer", "Full-time", "Lisbon"}, []string{"Lisbon"}},
		{"a URL is not a place", []string{"Acme", "Engineer", "https://acme.example"}, nil},
		{"nothing to go on", []string{"Acme", "Engineer"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, locationsFromFields(tt.fields))
		})
	}
}

// TestPlainHeaderField covers the company names measured in the September
// 2026 thread, where a homepage written beside the name became part of it.
func TestPlainHeaderField(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Freeform ([http://freeform.co](http://freeform.co))", "Freeform"},
		{"Snout [https://snout.com/](https://snout.com/)", "Snout"},
		{"Cerity Partners ([https://ceritypartners.com](https://ceritypartners.com))", "Cerity Partners"},
		{"Acme (https://acme.example)", "Acme"},
		{"[Acme](https://acme.example)", "Acme"},
		{"  Remote (Europe)  ", "Remote (Europe)"},
		{"Open Education / Neon", "Open Education / Neon"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, plainHeaderField(tt.in), tt.in)
	}
}

func TestHackerNewsSource_CompanyWrittenWithItsHomepage(t *testing.T) {
	thread := `{"id": 3, "title": "Ask HN: Who is hiring? (September 2026)", "children": [
		{"id": 21, "created_at": "2026-09-01T16:00:00Z",
		 "text": "Freeform (<a href=\"http://freeform.co\">http://freeform.co</a>) | Software Engineers | Hawthorne, CA | Onsite | https://freeform.co/careers"}
	]}`
	source := NewHackerNewsSource(hnServer(t, hnStories, thread).URL)

	jobs, err := source.Search(context.Background(), domain.SearchCriteria{})
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	assert.Equal(t, "Freeform", jobs[0].CompanyName)
	assert.Equal(t, "Software Engineers", jobs[0].Title)
}
