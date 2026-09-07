//go:build integration

package sqlite

import (
	"context"
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

	jobs, err := repo.List(context.Background(), 10, 0)

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

	jobs, err := repo.List(context.Background(), 1, 1)

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
