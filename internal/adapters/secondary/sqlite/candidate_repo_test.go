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
	"github.com/unicdev/jobpilot/internal/core/services"
)

func TestCandidateRepository_SaveAndGetByID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewCandidateRepository(db.Conn())
	candidate := &domain.Candidate{FirstName: "Taiwo", Email: "taiwo@example.com", Summary: "Go engineer"}

	require.NoError(t, repo.Save(context.Background(), candidate))
	assert.NotEmpty(t, candidate.ID, "Save should generate an ID")
	assert.False(t, candidate.CreatedAt.IsZero())

	got, err := repo.GetByID(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, "Taiwo", got.FirstName)
	assert.Equal(t, "Go engineer", got.Summary)
}

func TestCandidateRepository_GetByID_NotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewCandidateRepository(db.Conn())

	_, err := repo.GetByID(context.Background(), "does-not-exist")

	assert.ErrorContains(t, err, "candidate not found")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestCandidateRepository_Update(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewCandidateRepository(db.Conn())
	candidate := &domain.Candidate{FirstName: "Taiwo"}
	require.NoError(t, repo.Save(context.Background(), candidate))

	candidate.Summary = "Updated summary"
	require.NoError(t, repo.Update(context.Background(), candidate))

	got, err := repo.GetByID(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated summary", got.Summary)
}

func TestCandidateRepository_GetFirstCandidate(t *testing.T) {
	t.Parallel()

	t.Run("returns the first saved candidate", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		repo := NewCandidateRepository(db.Conn())
		candidate := &domain.Candidate{FirstName: "Taiwo"}
		require.NoError(t, repo.Save(context.Background(), candidate))

		got, err := repo.GetFirstCandidate(context.Background())

		require.NoError(t, err)
		assert.Equal(t, candidate.ID, got.ID)
	})

	t.Run("errors when no candidate exists", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		repo := NewCandidateRepository(db.Conn())

		_, err := repo.GetFirstCandidate(context.Background())

		assert.ErrorContains(t, err, "no candidate profile found")
	})
}

// TestCandidateRepository_LinksAndWorkAuthorizations covers the two tables
// ADR-0015 added, which exist because a form asks for these facts one at a
// time and one string could only ever give it the same answer twice.
func TestCandidateRepository_LinksAndWorkAuthorizations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	notice := 30

	t.Run("round-trips with the candidate", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		repo := NewCandidateRepository(db.Conn())

		candidate := &domain.Candidate{
			FirstName:        "Ada",
			Email:            "ada@example.com",
			Phone:            "+49 30 123456",
			LocationCity:     "Berlin",
			LocationCountry:  "Germany",
			TimeZone:         "Europe/Berlin",
			NoticePeriodDays: &notice,
			Links: []domain.CandidateLink{
				{Kind: domain.LinkKindLinkedIn, URL: "https://linkedin.com/in/ada"},
				{Kind: domain.LinkKindGitHub, URL: "https://github.com/ada"},
			},
			WorkAuthorizations: []domain.WorkAuthorization{
				{Country: "Germany", IsAuthorized: true, RequiresSponsorship: false},
				{Country: "United Kingdom", IsAuthorized: false, RequiresSponsorship: true},
			},
		}
		require.NoError(t, repo.Save(ctx, candidate))

		got, err := repo.GetByID(ctx, candidate.ID)
		require.NoError(t, err)

		assert.Equal(t, "ada@example.com", got.Email)
		assert.Equal(t, "+49 30 123456", got.Phone, "email and phone must not be the same field again")
		assert.Equal(t, "Europe/Berlin", got.TimeZone)
		require.NotNil(t, got.NoticePeriodDays)
		assert.Equal(t, 30, *got.NoticePeriodDays)

		linkedIn, ok := got.LinkOf(domain.LinkKindLinkedIn)
		require.True(t, ok)
		assert.Equal(t, "https://linkedin.com/in/ada", linkedIn.URL)
		gitHub, ok := got.LinkOf(domain.LinkKindGitHub)
		require.True(t, ok)
		assert.Equal(t, "https://github.com/ada", gitHub.URL)
		_, ok = got.LinkOf(domain.LinkKindTwitter)
		assert.False(t, ok, "a link that was never given must not come back")

		require.Len(t, got.WorkAuthorizations, 2)
		assert.Equal(t, "Germany", got.WorkAuthorizations[0].Country, "ordered by country")
		assert.True(t, got.WorkAuthorizations[0].IsAuthorized)
		assert.True(t, got.WorkAuthorizations[1].RequiresSponsorship)
	})

	t.Run("an update replaces them rather than accumulating", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		repo := NewCandidateRepository(db.Conn())

		candidate := &domain.Candidate{
			FirstName: "Ada",
			Links: []domain.CandidateLink{
				{Kind: domain.LinkKindLinkedIn, URL: "https://linkedin.com/in/old"},
				{Kind: domain.LinkKindGitHub, URL: "https://github.com/ada"},
			},
			WorkAuthorizations: []domain.WorkAuthorization{
				{Country: "Germany", IsAuthorized: true},
			},
		}
		require.NoError(t, repo.Save(ctx, candidate))

		// The user edits the form: one link changed, one removed, and the
		// authorization dropped entirely.
		candidate.Links = []domain.CandidateLink{
			{Kind: domain.LinkKindLinkedIn, URL: "https://linkedin.com/in/new"},
		}
		candidate.WorkAuthorizations = nil
		require.NoError(t, repo.Update(ctx, candidate))

		got, err := repo.GetByID(ctx, candidate.ID)
		require.NoError(t, err)

		require.Len(t, got.Links, 1)
		assert.Equal(t, "https://linkedin.com/in/new", got.Links[0].URL)
		assert.Empty(t, got.WorkAuthorizations, "a removed country must not keep answering for the user")
	})

	t.Run("deleting the candidate takes them with it", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		repo := NewCandidateRepository(db.Conn())

		candidate := &domain.Candidate{
			FirstName:          "Ada",
			Links:              []domain.CandidateLink{{Kind: domain.LinkKindWebsite, URL: "https://ada.dev"}},
			WorkAuthorizations: []domain.WorkAuthorization{{Country: "Germany", IsAuthorized: true}},
		}
		require.NoError(t, repo.Save(ctx, candidate))

		svc := services.NewDeletionService(NewDeletionStore(db), noCredentials{}, nil, nil)
		reach, err := svc.Preview(ctx, domain.DeleteProfile, "")
		require.NoError(t, err)
		assert.Equal(t, 2, reach.Deleted[domain.RecordCandidateDetail], "the preview counts the profile's details")
		_, err = svc.Delete(ctx, domain.DeleteProfile, "", reach.Token)
		require.NoError(t, err)

		for _, table := range []string{"candidate_links", "candidate_work_authorizations"} {
			var count int
			require.NoError(t, db.Conn().QueryRowContext(ctx,
				`SELECT COUNT(*) FROM `+table+` WHERE candidate_id = ?`, candidate.ID).Scan(&count))
			assert.Zero(t, count, "%s left an orphaned row", table)
		}
	})
}

// TestCandidateRepository_SkillsAndExperiences covers the two tables
// ADR-0015 stage two added, which exist to make "Current company" and a
// countable years of experience possible at all.
func TestCandidateRepository_SkillsAndExperiences(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	years := 4.0
	start := domain.NewDate(2022, time.July, 1)
	oldStart := domain.NewDate(2016, time.September, 21)
	oldEnd := domain.NewDate(2022, time.June, 30)

	db := newTestDB(t)
	repo := NewCandidateRepository(db.Conn())

	candidate := &domain.Candidate{
		FirstName: "Ada",
		Skills: []domain.CandidateSkill{
			{Name: "Go", YearsExperience: &years},
			{Name: "TypeScript"},
		},
		Experiences: []domain.CandidateExperience{
			{Company: "Old Co", Title: "Engineer", StartDate: &oldStart, EndDate: &oldEnd},
			{Company: "Acme", Title: "Staff Engineer", StartDate: &start, IsCurrent: true,
				Description: "Led the payments rewrite."},
		},
	}
	require.NoError(t, repo.Save(ctx, candidate))

	got, err := repo.GetByID(ctx, candidate.ID)
	require.NoError(t, err)

	require.Len(t, got.Skills, 2)
	goSkill, ok := got.SkillNamed("Go")
	require.True(t, ok)
	require.NotNil(t, goSkill.YearsExperience)
	assert.Equal(t, 4.0, *goSkill.YearsExperience)
	typescript, ok := got.SkillNamed("TypeScript")
	require.True(t, ok)
	assert.Nil(t, typescript.YearsExperience, "an unstated duration must stay unstated, not become zero")

	require.Len(t, got.Experiences, 2)
	assert.Equal(t, "Acme", got.Experiences[0].Company, "the current role comes first")
	assert.True(t, got.Experiences[0].IsCurrent)
	require.NotNil(t, got.Experiences[0].StartDate)
	assert.Equal(t, 2022, got.Experiences[0].StartDate.Year())
	assert.Nil(t, got.Experiences[0].EndDate)
	assert.Equal(t, "Led the payments rewrite.", got.Experiences[0].Description)

	current, ok := got.CurrentExperience()
	require.True(t, ok)
	assert.Equal(t, "Acme", current.Company)

	total, ok := got.TotalYearsOfExperience(time.Date(2026, time.September, 21, 0, 0, 0, 0, time.UTC))
	require.True(t, ok)
	assert.InDelta(t, 10.0, total, 0.05)
}
