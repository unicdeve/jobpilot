package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(year int, month time.Month, dayOfMonth int) *time.Time {
	t := time.Date(year, month, dayOfMonth, 0, 0, 0, 0, time.UTC)
	return &t
}

func TestCandidate_CurrentExperience(t *testing.T) {
	t.Parallel()

	t.Run("the flag wins, because the candidate said so directly", func(t *testing.T) {
		t.Parallel()
		c := Candidate{Experiences: []CandidateExperience{
			{Company: "Old Co", Title: "Engineer", StartDate: day(2019, time.January, 1), EndDate: day(2022, time.June, 30)},
			{Company: "Acme", Title: "Staff Engineer", StartDate: day(2022, time.July, 1), IsCurrent: true},
		}}

		got, ok := c.CurrentExperience()

		require.True(t, ok)
		assert.Equal(t, "Acme", got.Company)
	})

	t.Run("without a flag, the open-ended job with the latest start", func(t *testing.T) {
		t.Parallel()
		c := Candidate{Experiences: []CandidateExperience{
			{Company: "Old Co", StartDate: day(2015, time.January, 1)},
			{Company: "Acme", StartDate: day(2022, time.July, 1)},
			{Company: "Finished Co", StartDate: day(2023, time.January, 1), EndDate: day(2024, time.January, 1)},
		}}

		got, ok := c.CurrentExperience()

		require.True(t, ok)
		assert.Equal(t, "Acme", got.Company, "a job with an end date is not the current one, however recent")
	})

	t.Run("a history of only finished jobs has no current one", func(t *testing.T) {
		t.Parallel()
		c := Candidate{Experiences: []CandidateExperience{
			{Company: "Old Co", StartDate: day(2015, time.January, 1), EndDate: day(2020, time.January, 1)},
		}}

		_, ok := c.CurrentExperience()

		assert.False(t, ok, "between jobs is a real state, and guessing an employer would invent a fact")
	})

	t.Run("an empty history has no current job", func(t *testing.T) {
		t.Parallel()
		_, ok := Candidate{}.CurrentExperience()
		assert.False(t, ok)
	})
}

func TestCandidate_TotalYearsOfExperience(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.September, 21, 0, 0, 0, 0, time.UTC)

	t.Run("measures the span from the first job to now while one is current", func(t *testing.T) {
		t.Parallel()
		c := Candidate{Experiences: []CandidateExperience{
			{Company: "Old Co", StartDate: day(2016, time.September, 21), EndDate: day(2022, time.June, 30)},
			{Company: "Acme", StartDate: day(2022, time.July, 1), IsCurrent: true},
		}}

		years, ok := c.TotalYearsOfExperience(now)

		require.True(t, ok)
		assert.InDelta(t, 10.0, years, 0.05)
	})

	t.Run("overlapping jobs are not counted twice", func(t *testing.T) {
		t.Parallel()
		// Two roles held at once for four years is four years of
		// experience, not eight.
		c := Candidate{Experiences: []CandidateExperience{
			{Company: "Day Job", StartDate: day(2022, time.September, 21), EndDate: day(2026, time.September, 21)},
			{Company: "Evening Contract", StartDate: day(2022, time.September, 21), EndDate: day(2026, time.September, 21)},
		}}

		years, ok := c.TotalYearsOfExperience(now)

		require.True(t, ok)
		assert.InDelta(t, 4.0, years, 0.05)
	})

	t.Run("a finished history stops at the last end date", func(t *testing.T) {
		t.Parallel()
		c := Candidate{Experiences: []CandidateExperience{
			{Company: "Old Co", StartDate: day(2010, time.September, 21), EndDate: day(2015, time.September, 21)},
		}}

		years, ok := c.TotalYearsOfExperience(now)

		require.True(t, ok)
		assert.InDelta(t, 5.0, years, 0.05, "a career break is not experience")
	})

	t.Run("entries without dates cannot be counted", func(t *testing.T) {
		t.Parallel()
		c := Candidate{Experiences: []CandidateExperience{{Company: "Acme", Title: "Engineer"}}}

		_, ok := c.TotalYearsOfExperience(now)

		assert.False(t, ok, "a number invented from nothing is exactly what must not reach a form")
	})

	t.Run("no history at all", func(t *testing.T) {
		t.Parallel()
		_, ok := Candidate{}.TotalYearsOfExperience(now)
		assert.False(t, ok)
	})
}

func TestCandidateExperience_Period(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   CandidateExperience
		want string
	}{
		{"a finished role", CandidateExperience{StartDate: day(2019, time.March, 1), EndDate: day(2022, time.June, 1)}, "Mar 2019 – Jun 2022"},
		{"the current role", CandidateExperience{StartDate: day(2022, time.July, 1), IsCurrent: true}, "Jul 2022 – present"},
		{"open-ended without the flag", CandidateExperience{StartDate: day(2022, time.July, 1)}, "Jul 2022 – present"},
		{"only an end date", CandidateExperience{EndDate: day(2022, time.June, 1)}, "until Jun 2022"},
		{"no dates at all", CandidateExperience{Company: "Acme"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.in.Period())
		})
	}
}

func TestCandidate_RenderExperiences(t *testing.T) {
	t.Parallel()

	c := Candidate{Experiences: []CandidateExperience{
		{
			Company: "Acme", Title: "Staff Engineer",
			StartDate: day(2022, time.July, 1), IsCurrent: true,
			Description: "Led the payments rewrite.",
		},
		{Company: "Old Co", Title: "Engineer", StartDate: day(2019, time.March, 1), EndDate: day(2022, time.June, 1)},
	}}

	got := c.RenderExperiences()

	assert.Contains(t, got, "- Staff Engineer at Acme (Jul 2022 – present)")
	assert.Contains(t, got, "  Led the payments rewrite.")
	assert.Contains(t, got, "- Engineer at Old Co (Mar 2019 – Jun 2022)")
	assert.NotContains(t, got, "{", "a prompt must never receive a Go struct dump")
}

func TestCandidate_SkillHelpers(t *testing.T) {
	t.Parallel()

	years := 4.0
	c := Candidate{Skills: []CandidateSkill{
		{Name: "Go", YearsExperience: &years},
		{Name: "TypeScript"},
	}}

	assert.Equal(t, "Go, TypeScript", c.RenderSkills())

	got, ok := c.SkillNamed("go")
	require.True(t, ok, "a skill is the same skill however it is capitalised")
	require.NotNil(t, got.YearsExperience)
	assert.Equal(t, 4.0, *got.YearsExperience)

	_, ok = c.SkillNamed("Rust")
	assert.False(t, ok)
}
