package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func matchingScout() Scout {
	return Scout{
		Name: "Daily Go", SavedSearchID: "search-1", MinMatchScore: 70,
		MaxApplicationsPerRun: 5, Source: ScoutSourceLocal, IsEnabled: true,
	}
}

func TestScout_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		adjust  func(*Scout)
		wantErr string
	}{
		{"a usable scout", func(*Scout) {}, ""},
		{"no name", func(s *Scout) { s.Name = "   " }, "needs a name"},
		{"no saved search", func(s *Scout) { s.SavedSearchID = "" }, "needs a saved search"},
		{"unknown source", func(s *Scout) { s.Source = "browser" }, "unknown scout source"},
		{"negative score floor", func(s *Scout) { s.MinMatchScore = -1 }, "between 0 and 100"},
		{"impossible score floor", func(s *Scout) { s.MinMatchScore = 101 }, "between 0 and 100"},
		{"a run that can create nothing", func(s *Scout) { s.MaxApplicationsPerRun = 0 }, "between 1 and 50"},
		{"a run with no real ceiling", func(s *Scout) { s.MaxApplicationsPerRun = 5000 }, "between 1 and 50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			scout := matchingScout()
			tt.adjust(&scout)

			err := scout.Validate()

			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestScout_Eligible(t *testing.T) {
	t.Parallel()

	goJob := &Job{ID: "job-1", Title: "Senior Go Engineer", CompanyName: "Acme"}
	criteria := SearchCriteria{Keywords: "Go"}

	t.Run("a matching job that clears the floor is eligible, and says why", func(t *testing.T) {
		t.Parallel()
		ok, reason := matchingScout().Eligible(EligibilityInput{
			Job: goJob, Criteria: criteria, Score: 85, HasScore: true,
		})

		assert.True(t, ok)
		assert.Contains(t, reason, "85")
		assert.Contains(t, reason, "70", "the reason names the floor it cleared")
	})

	t.Run("a job the search excludes is rejected before anything else", func(t *testing.T) {
		t.Parallel()
		ok, reason := matchingScout().Eligible(EligibilityInput{
			Job:      &Job{ID: "job-2", Title: "Frontend Engineer"},
			Criteria: criteria, Score: 99, HasScore: true,
		})

		assert.False(t, ok)
		assert.Equal(t, "does not match the saved search", reason,
			"a high score does not override the search: the user chose the search")
	})

	t.Run("a job already applied to is rejected", func(t *testing.T) {
		t.Parallel()
		ok, reason := matchingScout().Eligible(EligibilityInput{
			Job: goJob, Criteria: criteria, Score: 99, HasScore: true, AlreadyApplied: true,
		})

		assert.False(t, ok)
		assert.Contains(t, reason, "already exists")
	})

	t.Run("a job that could not be scored is not treated as scoring zero", func(t *testing.T) {
		t.Parallel()
		// Nor as clearing the floor. A job nobody could judge is not a job
		// judged badly, and either default would be a fact invented about
		// the fit.
		ok, reason := matchingScout().Eligible(EligibilityInput{
			Job: goJob, Criteria: criteria, HasScore: false,
		})

		assert.False(t, ok)
		assert.Contains(t, reason, "could not be scored")
	})

	t.Run("a score below the floor is rejected and the numbers are named", func(t *testing.T) {
		t.Parallel()
		ok, reason := matchingScout().Eligible(EligibilityInput{
			Job: goJob, Criteria: criteria, Score: 69, HasScore: true,
		})

		assert.False(t, ok)
		assert.Contains(t, reason, "69")
		assert.Contains(t, reason, "70")
	})

	t.Run("a score exactly on the floor clears it", func(t *testing.T) {
		t.Parallel()
		ok, _ := matchingScout().Eligible(EligibilityInput{
			Job: goJob, Criteria: criteria, Score: 70, HasScore: true,
		})
		assert.True(t, ok, "a floor is a minimum, not something to exceed")
	})

	t.Run("the ceiling stops a run that has made its quota", func(t *testing.T) {
		t.Parallel()
		ok, reason := matchingScout().Eligible(EligibilityInput{
			Job: goJob, Criteria: criteria, Score: 99, HasScore: true, CreatedSoFar: 5,
		})

		assert.False(t, ok)
		assert.Contains(t, reason, "limit of 5")
	})

	t.Run("a floor of zero admits everything the search matched", func(t *testing.T) {
		t.Parallel()
		scout := matchingScout()
		scout.MinMatchScore = 0

		ok, _ := scout.Eligible(EligibilityInput{Job: goJob, Criteria: criteria, Score: 0, HasScore: true})

		assert.True(t, ok, "zero means the user wants to see everything the search found")
	})

	t.Run("a missing job is refused rather than dereferenced", func(t *testing.T) {
		t.Parallel()
		ok, reason := matchingScout().Eligible(EligibilityInput{Criteria: criteria, Score: 99, HasScore: true})
		assert.False(t, ok)
		assert.Contains(t, reason, "could not be read")
	})
}

func TestScout_PreScreenSkipsTheExpensiveCheck(t *testing.T) {
	t.Parallel()

	// PreScreen is everything that does not need a score, so a caller can
	// avoid a model call it does not need.
	ok, _ := matchingScout().PreScreen(EligibilityInput{
		Job:      &Job{ID: "job-1", Title: "Senior Go Engineer"},
		Criteria: SearchCriteria{Keywords: "Go"},
	})
	require.True(t, ok, "an unscored job passes the pre-screen; the score decides later")

	ok, reason := matchingScout().PreScreen(EligibilityInput{
		Job:      &Job{ID: "job-2", Title: "Frontend Engineer"},
		Criteria: SearchCriteria{Keywords: "Go"},
	})
	assert.False(t, ok)
	assert.Equal(t, "does not match the saved search", reason)
}

func TestScoutSource_IsValid(t *testing.T) {
	t.Parallel()

	assert.True(t, ScoutSourceLocal.IsValid())
	assert.True(t, ScoutSourceFetch.IsValid())
	assert.False(t, ScoutSource("").IsValid(), "a scout must say where its jobs come from")
	assert.False(t, ScoutSource("browser").IsValid(), "a scout stops before the browser")
}

// A job the user ruled out is passed over before anything else is asked of
// it, including whether it matches, so its reason is the user's own.
func TestScout_PreScreenPassesOverJobsRuledOut(t *testing.T) {
	t.Parallel()
	ruledOut := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)

	ok, reason := matchingScout().PreScreen(EligibilityInput{
		Job:      &Job{ID: "job-1", Title: "Senior Go Engineer", NotInterestedAt: &ruledOut},
		Criteria: SearchCriteria{Keywords: "Go"},
	})

	assert.False(t, ok)
	assert.Equal(t, "you marked this job not interested", reason)
}
