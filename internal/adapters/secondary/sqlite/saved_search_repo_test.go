//go:build integration

package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func goSearch(name string) *domain.SavedSearch {
	return &domain.SavedSearch{
		Name: name,
		Criteria: domain.SearchCriteria{
			Keywords:          "golang backend",
			Title:             "engineer",
			Location:          "remote",
			RemotePreference:  "remote",
			EmploymentType:    "full_time",
			SalaryMin:         100000,
			SalaryMax:         180000,
			ExcludedCompanies: []string{"Acme", "Globex"},
		},
	}
}

func TestSavedSearchRepository_SaveAndGetByID(t *testing.T) {
	t.Parallel()

	repo := NewSavedSearchRepository(newTestDB(t).Conn())
	ctx := context.Background()
	search := goSearch("Go roles")

	require.NoError(t, repo.Save(ctx, search))
	require.NotEmpty(t, search.ID)

	got, err := repo.GetByID(ctx, search.ID)
	require.NoError(t, err)
	assert.Equal(t, "Go roles", got.Name)
	assert.Equal(t, search.Criteria, got.Criteria, "every criterion should survive a round trip")
}

func TestSavedSearchRepository_GetByID_NotFound(t *testing.T) {
	t.Parallel()

	_, err := NewSavedSearchRepository(newTestDB(t).Conn()).GetByID(context.Background(), "nope")

	assert.ErrorContains(t, err, "saved search not found")
}

func TestSavedSearchRepository_GetByName(t *testing.T) {
	t.Parallel()

	repo := NewSavedSearchRepository(newTestDB(t).Conn())
	ctx := context.Background()
	require.NoError(t, repo.Save(ctx, goSearch("Go roles")))

	found, err := repo.GetByName(ctx, "go ROLES")
	require.NoError(t, err)
	require.NotNil(t, found, "names should match regardless of case")
	assert.Equal(t, "Go roles", found.Name)

	// Absence is the expected answer when checking a name is free, not an error.
	missing, err := repo.GetByName(ctx, "Rust roles")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestSavedSearchRepository_Update(t *testing.T) {
	t.Parallel()

	repo := NewSavedSearchRepository(newTestDB(t).Conn())
	ctx := context.Background()
	search := goSearch("Go roles")
	require.NoError(t, repo.Save(ctx, search))

	search.Name = "Renamed"
	search.Criteria.Keywords = "rust"
	require.NoError(t, repo.Save(ctx, search))

	got, err := repo.GetByID(ctx, search.ID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", got.Name)
	assert.Equal(t, "rust", got.Criteria.Keywords)

	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1, "saving an existing search updates it rather than adding one")
}

func TestSavedSearchRepository_List_DefaultFirstThenByName(t *testing.T) {
	t.Parallel()

	repo := NewSavedSearchRepository(newTestDB(t).Conn())
	ctx := context.Background()
	for _, name := range []string{"Zebra roles", "Alpha roles"} {
		require.NoError(t, repo.Save(ctx, goSearch(name)))
	}
	preferred := goSearch("Middle roles")
	preferred.IsDefault = true
	require.NoError(t, repo.Save(ctx, preferred))

	searches, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, searches, 3)
	assert.Equal(t, "Middle roles", searches[0].Name, "the default belongs first")
	assert.Equal(t, "Alpha roles", searches[1].Name)
	assert.Equal(t, "Zebra roles", searches[2].Name)
}

// The schema permits one default, so promoting another must clear the first.
func TestSavedSearchRepository_SingleDefaultEnforced(t *testing.T) {
	t.Parallel()

	repo := NewSavedSearchRepository(newTestDB(t).Conn())
	ctx := context.Background()
	first := goSearch("Go roles")
	first.IsDefault = true
	require.NoError(t, repo.Save(ctx, first))

	second := goSearch("Rust roles")
	second.IsDefault = true
	err := repo.Save(ctx, second)
	assert.Error(t, err, "a second default must be refused by the schema, not silently allowed")

	require.NoError(t, repo.ClearDefault(ctx))
	require.NoError(t, repo.Save(ctx, second))

	searches, err := repo.List(ctx)
	require.NoError(t, err)
	defaults := 0
	for _, s := range searches {
		if s.IsDefault {
			defaults++
		}
	}
	assert.Equal(t, 1, defaults)
}

func TestSavedSearchRepository_NameIsUnique(t *testing.T) {
	t.Parallel()

	repo := NewSavedSearchRepository(newTestDB(t).Conn())
	ctx := context.Background()
	require.NoError(t, repo.Save(ctx, goSearch("Go roles")))

	err := repo.Save(ctx, goSearch("Go roles"))

	assert.Error(t, err, "duplicate names must be refused by the schema too, not only by the service")
}

func TestSavedSearchRepository_Delete(t *testing.T) {
	t.Parallel()

	repo := NewSavedSearchRepository(newTestDB(t).Conn())
	ctx := context.Background()
	search := goSearch("Go roles")
	require.NoError(t, repo.Save(ctx, search))

	require.NoError(t, repo.Delete(ctx, search.ID))

	_, err := repo.GetByID(ctx, search.ID)
	assert.ErrorContains(t, err, "not found")
}

func TestSavedSearchRepository_List_Empty(t *testing.T) {
	t.Parallel()

	searches, err := NewSavedSearchRepository(newTestDB(t).Conn()).List(context.Background())

	require.NoError(t, err)
	assert.Empty(t, searches)
	assert.NotNil(t, searches, "an empty list serializes as [] rather than null")
}
