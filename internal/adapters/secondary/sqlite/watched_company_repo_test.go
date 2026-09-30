//go:build integration

package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

func gitlab() *domain.WatchedCompany {
	return &domain.WatchedCompany{
		Board:     domain.Board{Provider: domain.ProviderGreenhouse, Name: "gitlab"},
		Name:      "GitLab",
		AddedFrom: "https://job-boards.greenhouse.io/gitlab/jobs/8556658002",
	}
}

func TestWatchedCompanyRepository_CreateAndGet(t *testing.T) {
	t.Parallel()

	repo := NewWatchedCompanyRepository(newTestDB(t).Conn())
	ctx := context.Background()

	created, err := repo.Create(ctx, gitlab())
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)
	assert.Equal(t, "GitLab", created.Name)
	assert.Equal(t, "https://job-boards.greenhouse.io/gitlab", created.URL)
	assert.Equal(t, domain.BoardCheck(""), created.Check, "not checked yet")
	assert.Nil(t, created.CheckedAt)

	got, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.Board, got.Board)
	assert.Equal(t, created.AddedFrom, got.AddedFrom)
}

func TestWatchedCompanyRepository_CreateTwiceKeepsTheFirst(t *testing.T) {
	t.Parallel()

	repo := NewWatchedCompanyRepository(newTestDB(t).Conn())
	ctx := context.Background()

	first, err := repo.Create(ctx, gitlab())
	require.NoError(t, err)

	again := gitlab()
	again.Board.Name = "GitLab"
	again.Name = "Another name"
	second, err := repo.Create(ctx, again)
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID, "a board's name is not case sensitive")
	assert.Equal(t, "GitLab", second.Name)

	eu := gitlab()
	eu.Board.Region = domain.RegionEU
	other, err := repo.Create(ctx, eu)
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, other.ID, "a board on the EU servers is another account")

	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestWatchedCompanyRepository_GetByBoard(t *testing.T) {
	t.Parallel()

	repo := NewWatchedCompanyRepository(newTestDB(t).Conn())
	ctx := context.Background()
	created, err := repo.Create(ctx, gitlab())
	require.NoError(t, err)

	found, err := repo.GetByBoard(ctx, domain.Board{Provider: domain.ProviderGreenhouse, Name: "GITLAB"})
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)

	missing, err := repo.GetByBoard(ctx, domain.Board{Provider: domain.ProviderLever, Name: "gitlab"})
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestWatchedCompanyRepository_ListByName(t *testing.T) {
	t.Parallel()

	repo := NewWatchedCompanyRepository(newTestDB(t).Conn())
	ctx := context.Background()
	for _, name := range []string{"zapier", "Airbnb", "clickup"} {
		_, err := repo.Create(ctx, &domain.WatchedCompany{
			Board: domain.Board{Provider: domain.ProviderAshby, Name: name}, Name: name, AddedFrom: name,
		})
		require.NoError(t, err)
	}

	all, err := repo.List(ctx)
	require.NoError(t, err)
	names := make([]string, len(all))
	for i, company := range all {
		names[i] = company.Name
	}
	assert.Equal(t, []string{"Airbnb", "clickup", "zapier"}, names)
}

func TestWatchedCompanyRepository_RenameCheckDelete(t *testing.T) {
	t.Parallel()

	repo := NewWatchedCompanyRepository(newTestDB(t).Conn())
	ctx := context.Background()
	created, err := repo.Create(ctx, gitlab())
	require.NoError(t, err)

	require.NoError(t, repo.Rename(ctx, created.ID, "GitLab Inc."))
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, repo.RecordCheck(ctx, created.ID, at, domain.BoardCheckGone, 0))

	got, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "GitLab Inc.", got.Name)
	assert.Equal(t, domain.BoardCheckGone, got.Check)
	require.NotNil(t, got.CheckedAt)
	assert.True(t, at.Equal(*got.CheckedAt))

	require.NoError(t, repo.RecordCheck(ctx, created.ID, at, domain.BoardCheckOK, 202))
	got, err = repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, 202, got.OpenRoles)

	require.NoError(t, repo.Delete(ctx, created.ID))
	_, err = repo.GetByID(ctx, created.ID)
	assert.ErrorIs(t, err, ports.ErrWatchedCompanyNotFound)
}

func TestWatchedCompanyRepository_UnknownID(t *testing.T) {
	t.Parallel()

	repo := NewWatchedCompanyRepository(newTestDB(t).Conn())
	ctx := context.Background()

	assert.ErrorIs(t, repo.Rename(ctx, "nope", "x"), ports.ErrWatchedCompanyNotFound)
	assert.ErrorIs(t, repo.Delete(ctx, "nope"), ports.ErrWatchedCompanyNotFound)
	assert.ErrorIs(t, repo.RecordCheck(ctx, "nope", time.Now(), domain.BoardCheckOK, 1), ports.ErrWatchedCompanyNotFound)
	_, err := repo.GetByID(ctx, "nope")
	assert.ErrorIs(t, err, ports.ErrWatchedCompanyNotFound)
}
