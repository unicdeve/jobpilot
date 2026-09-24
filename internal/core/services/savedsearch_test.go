package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func goCriteria() domain.SearchCriteria {
	return domain.SearchCriteria{Keywords: "golang backend"}
}

func TestSavedSearchService_Create(t *testing.T) {
	t.Parallel()

	t.Run("the first search becomes the default", func(t *testing.T) {
		t.Parallel()
		svc := NewSavedSearchService(newFakeSavedSearchRepository())

		first, err := svc.Create(context.Background(), "Go roles", goCriteria())
		require.NoError(t, err)
		assert.True(t, first.IsDefault, "a user who sets up one search should not also have to nominate it")

		second, err := svc.Create(context.Background(), "Design roles", domain.SearchCriteria{Keywords: "design"})
		require.NoError(t, err)
		assert.False(t, second.IsDefault)
	})

	t.Run("criteria that narrow nothing are rejected", func(t *testing.T) {
		t.Parallel()
		svc := NewSavedSearchService(newFakeSavedSearchRepository())

		_, err := svc.Create(context.Background(), "Everything", domain.SearchCriteria{})

		assert.ErrorContains(t, err, "at least one criterion")
	})

	t.Run("a search needs a name", func(t *testing.T) {
		t.Parallel()
		svc := NewSavedSearchService(newFakeSavedSearchRepository())

		_, err := svc.Create(context.Background(), "   ", goCriteria())

		assert.ErrorContains(t, err, "needs a name")
	})

	t.Run("names are unique regardless of case", func(t *testing.T) {
		t.Parallel()
		svc := NewSavedSearchService(newFakeSavedSearchRepository())
		_, err := svc.Create(context.Background(), "Go roles", goCriteria())
		require.NoError(t, err)

		_, err = svc.Create(context.Background(), "go ROLES", goCriteria())

		assert.ErrorContains(t, err, "already exists")
	})

	t.Run("the name is trimmed", func(t *testing.T) {
		t.Parallel()
		svc := NewSavedSearchService(newFakeSavedSearchRepository())

		search, err := svc.Create(context.Background(), "  Go roles  ", goCriteria())

		require.NoError(t, err)
		assert.Equal(t, "Go roles", search.Name)
	})
}

func TestSavedSearchService_Update(t *testing.T) {
	t.Parallel()

	svc := NewSavedSearchService(newFakeSavedSearchRepository())
	ctx := context.Background()
	created, err := svc.Create(ctx, "Go roles", goCriteria())
	require.NoError(t, err)

	t.Run("keeping its own name is not a clash", func(t *testing.T) {
		updated, err := svc.Update(ctx, created.ID, "Go roles", domain.SearchCriteria{Keywords: "rust"})
		require.NoError(t, err)
		assert.Equal(t, "rust", updated.Criteria.Keywords)
	})

	t.Run("default status survives an update", func(t *testing.T) {
		updated, err := svc.Update(ctx, created.ID, "Renamed", goCriteria())
		require.NoError(t, err)
		assert.True(t, updated.IsDefault)
	})

	t.Run("emptying the criteria is rejected", func(t *testing.T) {
		_, err := svc.Update(ctx, created.ID, "Renamed", domain.SearchCriteria{})
		assert.ErrorContains(t, err, "at least one criterion")
	})
}

func TestSavedSearchService_SetDefault(t *testing.T) {
	t.Parallel()

	svc := NewSavedSearchService(newFakeSavedSearchRepository())
	ctx := context.Background()
	first, err := svc.Create(ctx, "Go roles", goCriteria())
	require.NoError(t, err)
	second, err := svc.Create(ctx, "Rust roles", domain.SearchCriteria{Keywords: "rust"})
	require.NoError(t, err)

	promoted, err := svc.SetDefault(ctx, second.ID)
	require.NoError(t, err)
	assert.True(t, promoted.IsDefault)

	// Exactly one default: the previous holder must have been demoted.
	def, err := svc.Default(ctx)
	require.NoError(t, err)
	assert.Equal(t, second.ID, def.ID)

	demoted, err := svc.Get(ctx, first.ID)
	require.NoError(t, err)
	assert.False(t, demoted.IsDefault)
}

func TestSavedSearchService_Delete(t *testing.T) {
	t.Parallel()

	t.Run("deleting the default promotes another", func(t *testing.T) {
		t.Parallel()
		svc := NewSavedSearchService(newFakeSavedSearchRepository())
		ctx := context.Background()
		first, err := svc.Create(ctx, "Go roles", goCriteria())
		require.NoError(t, err)
		_, err = svc.Create(ctx, "Rust roles", domain.SearchCriteria{Keywords: "rust"})
		require.NoError(t, err)

		require.NoError(t, svc.Delete(ctx, first.ID))

		// Otherwise the user still has searches but an unqualified run fails
		// for a reason they would have to go looking for.
		def, err := svc.Default(ctx)
		require.NoError(t, err)
		require.NotNil(t, def)
		assert.Equal(t, "Rust roles", def.Name)
	})

	t.Run("deleting the last search leaves no default", func(t *testing.T) {
		t.Parallel()
		svc := NewSavedSearchService(newFakeSavedSearchRepository())
		ctx := context.Background()
		only, err := svc.Create(ctx, "Go roles", goCriteria())
		require.NoError(t, err)

		require.NoError(t, svc.Delete(ctx, only.ID))

		def, err := svc.Default(ctx)
		require.NoError(t, err)
		assert.Nil(t, def)
	})
}

func TestSavedSearchService_Default_NoneConfigured(t *testing.T) {
	t.Parallel()

	def, err := NewSavedSearchService(newFakeSavedSearchRepository()).Default(context.Background())

	require.NoError(t, err, "no saved searches is a state, not a failure")
	assert.Nil(t, def)
}

func TestSavedSearchService_RepositoryFailuresSurface(t *testing.T) {
	t.Parallel()

	repo := newFakeSavedSearchRepository()
	repo.listErr = errors.New("disk gone")

	_, err := NewSavedSearchService(repo).Create(context.Background(), "Go roles", goCriteria())

	assert.ErrorContains(t, err, "disk gone")
}

func TestSavedSearchService_Templates(t *testing.T) {
	t.Parallel()

	svc := NewSavedSearchService(newFakeSavedSearchRepository())
	groups := svc.Templates()

	require.NotEmpty(t, groups)
	assert.NotEmpty(t, groups[0].Templates)
}

func TestSavedSearchService_ImportTemplates(t *testing.T) {
	t.Parallel()

	t.Run("each template becomes an ordinary saved search", func(t *testing.T) {
		t.Parallel()
		svc := NewSavedSearchService(newFakeSavedSearchRepository())

		created, err := svc.ImportTemplates(context.Background(), []string{"backend-engineer", "fullstack-engineer"})
		require.NoError(t, err)
		require.Len(t, created, 2)

		assert.Equal(t, "Backend Engineer", created[0].Name)
		assert.Equal(t, "Full-stack Engineer", created[1].Name)
		assert.False(t, created[0].Criteria.IsEmpty())

		// They are listed and editable like any other search.
		listed, err := svc.List(context.Background())
		require.NoError(t, err)
		assert.Len(t, listed, 2)

		renamed, err := svc.Update(context.Background(), created[0].ID, "Backend, Berlin",
			domain.SearchCriteria{Title: "back end", Location: "berlin"})
		require.NoError(t, err)
		assert.Equal(t, "Backend, Berlin", renamed.Name)
	})

	t.Run("the first import into an empty profile becomes the default", func(t *testing.T) {
		t.Parallel()
		svc := NewSavedSearchService(newFakeSavedSearchRepository())

		created, err := svc.ImportTemplates(context.Background(), []string{"backend-engineer", "frontend-engineer"})
		require.NoError(t, err)

		assert.True(t, created[0].IsDefault, "a user whose first searches are imported still needs a default")
		assert.False(t, created[1].IsDefault)
	})

	t.Run("a second copy is numbered rather than refused", func(t *testing.T) {
		t.Parallel()
		svc := NewSavedSearchService(newFakeSavedSearchRepository())
		ctx := context.Background()

		first, err := svc.ImportTemplates(ctx, []string{"backend-engineer"})
		require.NoError(t, err)
		assert.Equal(t, "Backend Engineer", first[0].Name)

		second, err := svc.ImportTemplates(ctx, []string{"backend-engineer"})
		require.NoError(t, err)
		assert.Equal(t, "Backend Engineer 2", second[0].Name)

		third, err := svc.ImportTemplates(ctx, []string{"backend-engineer"})
		require.NoError(t, err)
		assert.Equal(t, "Backend Engineer 3", third[0].Name)
	})

	t.Run("a name already taken by hand is not overwritten", func(t *testing.T) {
		t.Parallel()
		svc := NewSavedSearchService(newFakeSavedSearchRepository())
		ctx := context.Background()

		mine, err := svc.Create(ctx, "backend engineer", domain.SearchCriteria{Keywords: "rust"})
		require.NoError(t, err)

		created, err := svc.ImportTemplates(ctx, []string{"backend-engineer"})
		require.NoError(t, err)
		assert.Equal(t, "Backend Engineer 2", created[0].Name,
			"names are compared without case, as SQLite compares them")

		unchanged, err := svc.Get(ctx, mine.ID)
		require.NoError(t, err)
		assert.Equal(t, "rust", unchanged.Criteria.Keywords)
	})

	t.Run("the same template twice in one request is imported once", func(t *testing.T) {
		t.Parallel()
		svc := NewSavedSearchService(newFakeSavedSearchRepository())

		created, err := svc.ImportTemplates(context.Background(),
			[]string{"backend-engineer", "backend-engineer"})

		require.NoError(t, err)
		assert.Len(t, created, 1)
	})

	t.Run("an unknown ID writes nothing", func(t *testing.T) {
		t.Parallel()
		svc := NewSavedSearchService(newFakeSavedSearchRepository())

		_, err := svc.ImportTemplates(context.Background(),
			[]string{"backend-engineer", "not-a-template"})

		assert.ErrorContains(t, err, "not-a-template")

		listed, err := svc.List(context.Background())
		require.NoError(t, err)
		assert.Empty(t, listed, "a mistyped ID should cost nothing, not half an import")
	})

	t.Run("choosing nothing is refused", func(t *testing.T) {
		t.Parallel()
		svc := NewSavedSearchService(newFakeSavedSearchRepository())

		_, err := svc.ImportTemplates(context.Background(), nil)

		assert.ErrorContains(t, err, "no templates chosen")
	})
}
