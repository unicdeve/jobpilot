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
