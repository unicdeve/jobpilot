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

	second.IsDefault = false
	require.NoError(t, repo.Save(ctx, second))
	_, err = repo.SetDefault(ctx, second.ID)
	require.NoError(t, err)

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

func TestSavedSearchRepository_MarkRun(t *testing.T) {
	t.Parallel()

	repo := NewSavedSearchRepository(newTestDB(t).Conn())
	ctx := context.Background()
	search := goSearch("Go roles")
	require.NoError(t, repo.Save(ctx, search))

	got, err := repo.GetByID(ctx, search.ID)
	require.NoError(t, err)
	assert.Nil(t, got.LastRunAt, "a new search has never run")
	updatedAt := got.UpdatedAt

	ranAt := time.Date(2026, 9, 29, 8, 30, 0, 0, time.UTC)
	require.NoError(t, repo.MarkRun(ctx, search.ID, ranAt))

	got, err = repo.GetByID(ctx, search.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastRunAt)
	assert.True(t, ranAt.Equal(*got.LastRunAt))
	assert.Equal(t, updatedAt, got.UpdatedAt, "running a search is not editing it")

	got.Name = "Renamed"
	require.NoError(t, repo.Save(ctx, got))
	got, err = repo.GetByID(ctx, search.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastRunAt, "editing a search keeps when it last ran")
	assert.True(t, ranAt.Equal(*got.LastRunAt))

	assert.NoError(t, repo.MarkRun(ctx, "gone", ranAt), "a search deleted meanwhile is not an error")
}

func TestSavedSearchRepository_DefaultChangeRollsBack(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"promote", "delete"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			db := newTestDB(t)
			repo := NewSavedSearchRepository(db.Conn())
			first := goSearch("Go roles")
			first.IsDefault = true
			second := goSearch("Rust roles")
			require.NoError(t, repo.Save(ctx, first))
			require.NoError(t, repo.Save(ctx, second))

			// Inject failure at promotion, after the old default has been
			// cleared or deleted. This trigger exists only in the test DB.
			_, err := db.Conn().ExecContext(ctx, `CREATE TRIGGER fail_promotion
				BEFORE UPDATE OF is_default ON saved_searches WHEN NEW.is_default = 1
				BEGIN SELECT RAISE(ABORT, 'promotion failed'); END`)
			require.NoError(t, err)
			if operation == "promote" {
				_, err = repo.SetDefault(ctx, second.ID)
			} else {
				err = repo.Delete(ctx, first.ID)
			}
			require.ErrorContains(t, err, "promotion failed")

			searches, err := repo.List(ctx)
			require.NoError(t, err)
			require.Len(t, searches, 2, "a failed deletion must restore the search too")
			assert.Equal(t, first.ID, searches[0].ID)
			assert.True(t, searches[0].IsDefault, "failure must preserve the original default")
			assert.False(t, searches[1].IsDefault)
		})
	}
}

func TestSavedSearchRepository_SetDefaultMissingTarget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := NewSavedSearchRepository(newTestDB(t).Conn())
	first := goSearch("Go roles")
	first.IsDefault = true
	require.NoError(t, repo.Save(ctx, first))

	_, err := repo.SetDefault(ctx, "missing")
	require.ErrorContains(t, err, "not found")
	got, err := repo.GetByID(ctx, first.ID)
	require.NoError(t, err)
	assert.True(t, got.IsDefault)
}

func TestSavedSearchRepository_DeletePreservesDefault(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		deleteName string
		wantName   string
	}{
		{name: "promotes alphabetically", deleteName: "Middle", wantName: "alpha"},
		{name: "keeps existing default", deleteName: "alpha", wantName: "Middle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			repo := NewSavedSearchRepository(newTestDB(t).Conn())
			var deleteID string
			for _, name := range []string{"Zebra", "Middle", "alpha"} {
				search := goSearch(name)
				search.IsDefault = name == "Middle"
				require.NoError(t, repo.Save(ctx, search))
				if name == tc.deleteName {
					deleteID = search.ID
				}
			}
			require.NoError(t, repo.Delete(ctx, deleteID))
			searches, err := repo.List(ctx)
			require.NoError(t, err)
			require.Len(t, searches, 2)
			assert.Equal(t, tc.wantName, searches[0].Name)
			assert.True(t, searches[0].IsDefault)
			assert.False(t, searches[1].IsDefault)
			require.NoError(t, repo.Delete(ctx, searches[1].ID))
			require.NoError(t, repo.Delete(ctx, searches[0].ID))
			searches, err = repo.List(ctx)
			require.NoError(t, err)
			assert.Empty(t, searches, "deleting the final default needs no replacement")
		})
	}
}

func TestSavedSearchRepository_EditDoesNotUndoDefaultChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := NewSavedSearchRepository(newTestDB(t).Conn())
	first, second := goSearch("Go roles"), goSearch("Rust roles")
	first.IsDefault = true
	require.NoError(t, repo.Save(ctx, first))
	require.NoError(t, repo.Save(ctx, second))
	_, err := repo.SetDefault(ctx, second.ID)
	require.NoError(t, err)

	// Both objects predate the promotion. Editing either must preserve the
	// current default rather than restore its stale is_default value.
	for _, search := range []*domain.SavedSearch{first, second} {
		search.Criteria.Keywords = "updated"
		require.NoError(t, repo.Save(ctx, search))
	}
	searches, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, searches, 2)
	assert.Equal(t, second.ID, searches[0].ID)
	assert.True(t, searches[0].IsDefault)
	assert.False(t, searches[1].IsDefault)
	for _, search := range searches {
		assert.Equal(t, "updated", search.Criteria.Keywords)
	}
}

func TestSavedSearchRepository_ConcurrentDefaultChanges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := NewSavedSearchRepository(newTestDB(t).Conn())
	first, second := goSearch("Go roles"), goSearch("Rust roles")
	first.IsDefault = true
	require.NoError(t, repo.Save(ctx, first))
	require.NoError(t, repo.Save(ctx, second))

	start := make(chan struct{})
	errs := make(chan error, 8)
	for i := range 8 {
		id := first.ID
		if i%2 == 0 {
			id = second.ID
		}
		go func() {
			<-start
			_, err := repo.SetDefault(ctx, id)
			errs <- err
		}()
	}
	close(start)
	for range 8 {
		assert.NoError(t, <-errs)
	}
	searches, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, searches, 2)
	assert.True(t, searches[0].IsDefault)
	assert.False(t, searches[1].IsDefault)
}
