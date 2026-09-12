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

	jobs, err := repo.List(context.Background(), "", 10, 0)

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

	jobs, err := repo.List(context.Background(), "", 1, 1)

	require.NoError(t, err)
	require.Len(t, jobs, 1)
}

func TestJobRepository_Delete(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewJobRepository(db.Conn())
	job := &domain.Job{CanonicalURL: "https://acme.example/job-1"}
	require.NoError(t, repo.Save(context.Background(), job))

	require.NoError(t, repo.Delete(context.Background(), job.ID))

	_, err := repo.GetByID(context.Background(), job.ID)
	assert.ErrorContains(t, err, "job not found")
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

			jobs, err := repo.List(ctx, tt.keywords, 10, 0)
			require.NoError(t, err)

			titles := make([]string, 0, len(jobs))
			for _, j := range jobs {
				titles = append(titles, j.Title)
			}
			assert.ElementsMatch(t, tt.wantTitle, titles)

			total, err := repo.Count(ctx, tt.keywords)
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
	jobs, err := repo.List(ctx, "'; DROP TABLE jobs; --", 10, 0)
	require.NoError(t, err)
	assert.Empty(t, jobs)

	remaining, err := repo.Count(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, 1, remaining, "the jobs table must still be intact")
}

func TestJobRepository_Count(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewJobRepository(db.Conn())
	ctx := context.Background()

	total, err := repo.Count(ctx, "")
	require.NoError(t, err)
	assert.Zero(t, total, "an empty database counts zero rather than failing")

	for i := range 3 {
		require.NoError(t, repo.Save(ctx, &domain.Job{
			CanonicalURL: fmt.Sprintf("https://acme.example/job-%d", i),
			Title:        "Engineer",
		}))
	}

	total, err = repo.Count(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, 3, total)

	// Count describes the whole match, independent of any page size.
	page, err := repo.List(ctx, "", 1, 0)
	require.NoError(t, err)
	assert.Len(t, page, 1)
	assert.Equal(t, 3, total)
}
