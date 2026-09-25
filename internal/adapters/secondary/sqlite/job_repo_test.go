//go:build integration

package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func TestJobRepository_SaveAndGetByID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewJobRepository(db.Conn())
	job := &domain.Job{
		CanonicalURL: "https://acme.example/job-1",
		Title:        "Backend Engineer",
		CompanyName:  "Acme",
		Locations:    []string{"Lagos", "Remote"},
		Skills:       []string{"go", "sql"},
	}

	require.NoError(t, repo.Save(context.Background(), job))
	assert.NotEmpty(t, job.ID)

	got, err := repo.GetByID(context.Background(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, "Backend Engineer", got.Title)
	assert.Equal(t, []string{"Lagos", "Remote"}, got.Locations)
	assert.Equal(t, []string{"go", "sql"}, got.Skills)
}

func TestJobRepository_GetByID_NotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewJobRepository(db.Conn())

	_, err := repo.GetByID(context.Background(), "does-not-exist")

	assert.ErrorContains(t, err, "job not found")
}

func TestJobRepository_GetByCanonicalURL(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		repo := NewJobRepository(db.Conn())
		job := &domain.Job{CanonicalURL: "https://acme.example/job-1"}
		require.NoError(t, repo.Save(context.Background(), job))

		got, err := repo.GetByCanonicalURL(context.Background(), "https://acme.example/job-1")

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, job.ID, got.ID)
	})

	t.Run("not found returns nil, nil", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		repo := NewJobRepository(db.Conn())

		got, err := repo.GetByCanonicalURL(context.Background(), "https://acme.example/missing")

		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

func TestJobRepository_List(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewJobRepository(db.Conn())
	older := &domain.Job{CanonicalURL: "https://acme.example/older", DiscoveredAt: time.Now().Add(-time.Hour)}
	newer := &domain.Job{CanonicalURL: "https://acme.example/newer", DiscoveredAt: time.Now()}
	require.NoError(t, repo.Save(context.Background(), older))
	require.NoError(t, repo.Save(context.Background(), newer))

	jobs, err := repo.List(context.Background(), domain.JobFilter{}, 10, 0)

	require.NoError(t, err)
	require.Len(t, jobs, 2)
	assert.Equal(t, newer.ID, jobs[0].ID, "most recently discovered job should come first")
	assert.Equal(t, older.ID, jobs[1].ID)
}

func TestJobRepository_List_Pagination(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewJobRepository(db.Conn())
	for i := range 3 {
		require.NoError(t, repo.Save(context.Background(), &domain.Job{
			CanonicalURL: "https://acme.example/job-" + string(rune('a'+i)),
			DiscoveredAt: time.Now().Add(time.Duration(i) * time.Minute),
		}))
	}

	jobs, err := repo.List(context.Background(), domain.JobFilter{}, 1, 1)

	require.NoError(t, err)
	require.Len(t, jobs, 1)
}

func TestJobRepository_List_Keywords(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewJobRepository(db.Conn())
	ctx := context.Background()

	require.NoError(t, repo.Save(ctx, &domain.Job{
		CanonicalURL: "https://acme.example/go", Title: "Golang Engineer", CompanyName: "Acme",
	}))
	require.NoError(t, repo.Save(ctx, &domain.Job{
		CanonicalURL: "https://globex.example/ux", Title: "Product Designer", CompanyName: "Globex",
	}))

	tests := []struct {
		name      string
		keywords  string
		wantTitle []string
	}{
		{"empty keywords match everything", "", []string{"Product Designer", "Golang Engineer"}},
		{"matches title", "golang", []string{"Golang Engineer"}},
		{"matches company", "globex", []string{"Product Designer"}},
		{"is case insensitive", "GOLANG", []string{"Golang Engineer"}},
		{"matches a substring", "design", []string{"Product Designer"}},
		{"surrounding whitespace is ignored", "  golang  ", []string{"Golang Engineer"}},
		{"no match yields no rows", "kubernetes", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			jobs, err := repo.List(ctx, domain.JobFilter{Keywords: tt.keywords}, 10, 0)
			require.NoError(t, err)

			titles := make([]string, 0, len(jobs))
			for _, j := range jobs {
				titles = append(titles, j.Title)
			}
			assert.ElementsMatch(t, tt.wantTitle, titles)

			total, err := repo.Count(ctx, domain.JobFilter{Keywords: tt.keywords})
			require.NoError(t, err)
			assert.Equal(t, len(tt.wantTitle), total, "Count must agree with List")
		})
	}
}

func TestJobRepository_List_KeywordsAreNotTreatedAsSQL(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewJobRepository(db.Conn())
	ctx := context.Background()
	require.NoError(t, repo.Save(ctx, &domain.Job{
		CanonicalURL: "https://acme.example/go", Title: "Golang Engineer", CompanyName: "Acme",
	}))

	// Keywords come straight from a search box, so they are bound as
	// parameters rather than interpolated. The rows must survive.
	jobs, err := repo.List(ctx, domain.JobFilter{Keywords: "'; DROP TABLE jobs; --"}, 10, 0)
	require.NoError(t, err)
	assert.Empty(t, jobs)

	remaining, err := repo.Count(ctx, domain.JobFilter{})
	require.NoError(t, err)
	assert.Equal(t, 1, remaining, "the jobs table must still be intact")
}

func TestJobRepository_Count(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewJobRepository(db.Conn())
	ctx := context.Background()

	total, err := repo.Count(ctx, domain.JobFilter{})
	require.NoError(t, err)
	assert.Zero(t, total, "an empty database counts zero rather than failing")

	for i := range 3 {
		require.NoError(t, repo.Save(ctx, &domain.Job{
			CanonicalURL: fmt.Sprintf("https://acme.example/job-%d", i),
			Title:        "Engineer",
		}))
	}

	total, err = repo.Count(ctx, domain.JobFilter{})
	require.NoError(t, err)
	assert.Equal(t, 3, total)

	// Count describes the whole match, independent of any page size.
	page, err := repo.List(ctx, domain.JobFilter{}, 1, 0)
	require.NoError(t, err)
	assert.Len(t, page, 1)
	assert.Equal(t, 3, total)
}

// The stored-job search must agree with discovery about what a query means,
// so it matches terms in any order rather than the whole string.
func TestJobRepository_List_MatchesTermsInAnyOrder(t *testing.T) {
	t.Parallel()

	repo := NewJobRepository(newTestDB(t).Conn())
	ctx := context.Background()
	require.NoError(t, repo.Save(ctx, &domain.Job{
		CanonicalURL: "https://acme.example/1", Title: "Engineer, Backend (Go)", CompanyName: "Acme",
	}))
	require.NoError(t, repo.Save(ctx, &domain.Job{
		CanonicalURL: "https://globex.example/1", Title: "Product Designer", CompanyName: "Globex",
	}))

	tests := []struct {
		name     string
		keywords string
		want     int
	}{
		{"terms in the title's order", "engineer backend", 1},
		{"terms reversed", "backend engineer", 1},
		{"a term that is absent excludes the row", "backend rust", 0},
		{"terms may span title and company", "engineer acme", 1},
		{"whitespace collapses", "  backend   engineer  ", 1},
		{"no keywords match everything", "", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			jobs, err := repo.List(ctx, domain.JobFilter{Keywords: tt.keywords}, 10, 0)
			require.NoError(t, err)
			assert.Len(t, jobs, tt.want)

			total, err := repo.Count(ctx, domain.JobFilter{Keywords: tt.keywords})
			require.NoError(t, err)
			assert.Equal(t, tt.want, total, "Count must agree with List")
		})
	}
}

// Source, remote type and posting date narrow the list, alone and together,
// and Count agrees with List for each.
func TestJobRepository_List_Filters(t *testing.T) {
	t.Parallel()

	repo := NewJobRepository(newTestDB(t).Conn())
	ctx := context.Background()
	now := time.Now()

	jobs := []*domain.Job{
		// Posted two days ago, in a zone other than UTC: the comparison must
		// be of instants, not of the text.
		{CanonicalURL: "https://a.example/1", Title: "Go Engineer", SourceID: "arbeitnow", RemoteType: "remote",
			PostedAt: now.AddDate(0, 0, -2).In(time.FixedZone("WAT", 3600))},
		{CanonicalURL: "https://a.example/2", Title: "Designer", SourceID: "arbeitnow", RemoteType: "onsite",
			PostedAt: now.AddDate(0, 0, -20)},
		{CanonicalURL: "https://h.example/1", Title: "Go Engineer", SourceID: "hackernews", RemoteType: "hybrid",
			PostedAt: now.AddDate(0, 0, -40)},
		// No posting date: judged by when it was found, which is now.
		{CanonicalURL: "https://h.example/2", Title: "SRE", SourceID: "hackernews", RemoteType: "remote"},
	}
	for _, job := range jobs {
		require.NoError(t, repo.Save(ctx, job))
	}

	tests := []struct {
		name   string
		filter domain.JobFilter
		want   []string
	}{
		{"source", domain.JobFilter{SourceID: "hackernews"}, []string{"https://h.example/1", "https://h.example/2"}},
		{"remote type", domain.JobFilter{RemoteType: "remote"}, []string{"https://a.example/1", "https://h.example/2"}},
		{"posted within a week", domain.JobFilter{PostedSince: now.AddDate(0, 0, -7)},
			[]string{"https://a.example/1", "https://h.example/2"}},
		{"posted within a month", domain.JobFilter{PostedSince: now.AddDate(0, 0, -30)},
			[]string{"https://a.example/1", "https://a.example/2", "https://h.example/2"}},
		{"combined with keywords", domain.JobFilter{Keywords: "go", SourceID: "arbeitnow", PostedSince: now.AddDate(0, 0, -7)},
			[]string{"https://a.example/1"}},
		{"an unknown source matches nothing", domain.JobFilter{SourceID: "nowhere"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			listed, err := repo.List(ctx, tt.filter, 10, 0)
			require.NoError(t, err)
			var urls []string
			for _, job := range listed {
				urls = append(urls, job.CanonicalURL)
			}
			assert.ElementsMatch(t, tt.want, urls)

			total, err := repo.Count(ctx, tt.filter)
			require.NoError(t, err)
			assert.Equal(t, len(tt.want), total, "Count must agree with List")
		})
	}
}

// Times are written in UTC, in the form SQLite's date functions read, and
// come back as the same instant.
func TestJobRepository_StoresTimesInUTC(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewJobRepository(db.Conn())
	ctx := context.Background()

	posted := time.Date(2026, 9, 15, 19, 36, 10, 500000000, time.FixedZone("WAT", 3600))
	job := &domain.Job{CanonicalURL: "https://a.example/1", Title: "Engineer", PostedAt: posted}
	require.NoError(t, repo.Save(ctx, job))

	var stored string
	require.NoError(t, db.Conn().QueryRowContext(ctx,
		`SELECT CAST(posted_at AS TEXT) FROM jobs WHERE id = ?`, job.ID).Scan(&stored))
	assert.Equal(t, "2026-09-15 18:36:10.5+00:00", stored)

	got, err := repo.GetByID(ctx, job.ID)
	require.NoError(t, err)
	assert.True(t, got.PostedAt.Equal(posted))
}

func TestJobRepository_NotInterested(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewJobRepository(db.Conn())
	for _, id := range []string{"kept", "first-out", "second-out"} {
		require.NoError(t, repo.Save(ctx, &domain.Job{ID: id, CanonicalURL: "https://acme.example/" + id, Title: "Engineer"}))
	}
	earlier := time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)
	later := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	require.NoError(t, repo.SetNotInterested(ctx, "first-out", &earlier))
	require.NoError(t, repo.SetNotInterested(ctx, "second-out", &later))

	ids := func(filter domain.JobFilter) []string {
		jobs, err := repo.List(ctx, filter, 50, 0)
		require.NoError(t, err)
		var out []string
		for _, j := range jobs {
			out = append(out, j.ID)
		}
		return out
	}
	assert.Equal(t, []string{"kept"}, ids(domain.JobFilter{}))
	assert.Equal(t, []string{"second-out", "first-out"}, ids(domain.JobFilter{Interest: domain.JobsNotInterested}),
		"the most recently ruled out first")
	assert.Len(t, ids(domain.JobFilter{Interest: domain.JobsAll}), 3)

	total, err := repo.Count(ctx, domain.JobFilter{})
	require.NoError(t, err)
	assert.Equal(t, 1, total, "counts leave them out too")

	got, err := repo.GetByID(ctx, "first-out")
	require.NoError(t, err)
	require.NotNil(t, got.NotInterestedAt)
	assert.True(t, got.NotInterestedAt.Equal(earlier))

	require.NoError(t, repo.SetNotInterested(ctx, "first-out", nil))
	got, err = repo.GetByID(ctx, "first-out")
	require.NoError(t, err)
	assert.Nil(t, got.NotInterestedAt)

	assert.Error(t, repo.SetNotInterested(ctx, "no-such-job", &later))
}
