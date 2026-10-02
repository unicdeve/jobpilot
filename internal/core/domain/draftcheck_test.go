package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func years(n float64) *float64 { return &n }

// basisWith is a candidate with Go for four and a half years, Kubernetes
// with no duration, and a dated career of about six years.
func basisWith(ownWords ...string) DraftBasis {
	start := NewDate(2019, time.March, 1)
	candidate := Candidate{
		Summary: "Backend engineer.",
		Skills: []CandidateSkill{
			{Name: "Go", YearsExperience: years(4.5)},
			{Name: "Kubernetes"},
			{Name: "PostgreSQL", YearsExperience: years(3)},
		},
		Experiences: []CandidateExperience{{Company: "Acme", StartDate: &start, IsCurrent: true}},
	}
	return DraftBasisFor(candidate, time.Date(2025, time.March, 1, 0, 0, 0, 0, time.UTC), ownWords...)
}

func flagsOf(kind DraftFlagKind, flags []DraftFlag) []DraftFlag {
	var out []DraftFlag
	for _, flag := range flags {
		if flag.Kind == kind {
			out = append(out, flag)
		}
	}
	return out
}

// AC-001.
func TestCheckDraft_StockPhrases(t *testing.T) {
	t.Parallel()

	for text, want := range map[string]string{
		"I am excited to apply for the Backend Engineer role.":   "I am excited to apply",
		"I'm thrilled to apply for this role.":                   "I'm thrilled to apply",
		"i AM EXCITED TO APPLY.":                                 "i AM EXCITED TO APPLY",
		"I believe I would be a great fit for your team.":        "a great fit",
		"Thank you for your consideration.":                      "Thank you for your consideration",
		"I am writing to express my interest in the role.":       "I am writing to express my interest",
		"I’d welcome the opportunity to talk.":                   "I’d welcome the opportunity",
		"I look forward to hearing from you.":                    "I look forward to hearing from you",
		"I would be a valuable addition to\nyour platform team.": "a valuable addition to",
	} {
		t.Run(text, func(t *testing.T) {
			t.Parallel()
			got := flagsOf(FlagStockPhrase, CheckDraft(text, basisWith()))
			require.Len(t, got, 1)
			assert.Equal(t, want, got[0].Excerpt)
		})
	}

	t.Run("plain writing is not flagged", func(t *testing.T) {
		t.Parallel()
		text := "I rebuilt Acme's billing pipeline in Go, which cut failed charges by a third. Your post about idempotent retries is why I want this job."
		assert.Empty(t, CheckDraft(text, basisWith()))
	})
}

// AC-002.
func TestLetterWordLimit(t *testing.T) {
	t.Parallel()

	for preferences, want := range map[string]int{
		"Keep it under 200 words, plain and direct.": 200,
		"about 150 words":                     150,
		"150-200 words, no exclamation marks": 200,
		"Between 120 to 180 words.":           180,
		"Short and warm.":                     DefaultLetterWords,
		"":                                    DefaultLetterWords,
	} {
		limit, theirs := LetterWordLimit(preferences)
		assert.Equal(t, want, limit, preferences)
		assert.Equal(t, want != DefaultLetterWords, theirs, preferences)
	}
}

// AC-002.
func TestCheckDraft_Length(t *testing.T) {
	t.Parallel()

	words := func(n int) string { return strings.TrimSpace(strings.Repeat("word ", n)) }

	basis := basisWith()
	basis.WordLimit, basis.LimitIsTheirs = 200, true
	assert.Empty(t, CheckDraft(words(200), basis), "at the limit")

	got := flagsOf(FlagLength, CheckDraft(words(201), basis))
	require.Len(t, got, 1)
	assert.Equal(t, "201 words, over the 200 your writing preferences ask for.", got[0].Reason)
	assert.Empty(t, got[0].Excerpt)

	basis.WordLimit, basis.LimitIsTheirs = DefaultLetterWords, false
	got = flagsOf(FlagLength, CheckDraft(words(300), basis))
	require.Len(t, got, 1)
	assert.Equal(t, "300 words, over the 250 a letter is kept to.", got[0].Reason)

	basis.WordLimit = 0
	assert.Empty(t, CheckDraft(words(1000), basis), "an answer is never checked for length")
}

// AC-003 to AC-006.
func TestCheckDraft_YearsClaims(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		text    string
		own     []string
		excerpt string // empty when nothing is flagged
		reason  string
	}{
		// AC-003: a skill with a duration.
		{name: "within a skill's duration, rounded", text: "I have five years of Go experience."},
		{name: "over a skill's duration", text: "I bring 8+ years of Go.", excerpt: "8+ years", reason: "Claims 8 years of Go; you gave 4.5 years."},
		{name: "a qualifier is kept", text: "I have over six years with PostgreSQL.", excerpt: "over six years", reason: "Claims 6 years of PostgreSQL; you gave 3 years."},
		// AC-004: a skill with no duration.
		{name: "a skill with no duration", text: "I have run Kubernetes for 3 years.", excerpt: "3 years", reason: "Claims 3 years of Kubernetes, and you have not said how long you have used it."},
		{name: "a skill with no duration, in their own words", text: "I have run Kubernetes for 3 years.", own: []string{"Ran Kubernetes in production for 3 years at Acme."}},
		{name: "their own words about another skill do not count", text: "I have run Kubernetes for 3 years.", own: []string{"3 years of Terraform experience."}, excerpt: "3 years", reason: "Claims 3 years of Kubernetes, and you have not said how long you have used it."},
		// AC-005: no skill named.
		{name: "within the career", text: "I have six years of experience building payment systems."},
		{name: "over the career", text: "My ten years of experience in fintech taught me to test.", excerpt: "ten years", reason: "Claims 10 years; your work history covers about 6 years."},
		{name: "over the career, in their own words", text: "I bring 10 years of experience.", own: []string{"10+ years of experience in backend engineering."}},
		// AC-006: not claims.
		{name: "a time ago", text: "I moved to Lagos two years ago."},
		{name: "a time to come", text: "I want to grow into a staff role in the next five years."},
		{name: "an age", text: "I admire how your ten-year-old platform still ships weekly."},
		{name: "not about the candidate", text: "Acme has been profitable for 12 years."},
		{name: "the skill Go is not the verb go", text: "I have spent 9 years helping teams go faster.", excerpt: "9 years", reason: "Claims 9 years; your work history covers about 6 years."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := flagsOf(FlagUnsupportedClaim, CheckDraft(tt.text, basisWith(tt.own...)))
			if tt.excerpt == "" {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, tt.excerpt, got[0].Excerpt)
			assert.Equal(t, tt.reason, got[0].Reason)
		})
	}
}

// AC-005: no dated work history at all.
func TestCheckDraft_NoCareerToCheckAgainst(t *testing.T) {
	t.Parallel()

	basis := DraftBasisFor(Candidate{}, time.Now())
	got := flagsOf(FlagUnsupportedClaim, CheckDraft("I have 3 years of experience.", basis))
	require.Len(t, got, 1)
	assert.Equal(t, "Claims 3 years, and your work history has no dates to check it against.", got[0].Reason)
}

// The measured letter that prompted the check
// (docs/02_features/16_about_you.md): every problem is flagged once.
func TestCheckDraft_MeasuredLetter(t *testing.T) {
	t.Parallel()

	letter := "Dear Hiring Manager,\n\n" +
		"I am excited to apply for the AI Engineer position. With 8 years of experience building agents, I believe I would be a great fit.\n\n" +
		"I am excited to apply because your mission inspires me.\n\n" +
		"Thank you for your consideration."

	flags := CheckDraft(letter, basisWith())
	assert.Len(t, flagsOf(FlagStockPhrase, flags), 3, "the repeated opening is flagged once")
	assert.Len(t, flagsOf(FlagUnsupportedClaim, flags), 1)
}
