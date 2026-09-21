package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A draft comes from a model reading an untrusted document, so these tests
// are about what happens when it returns something other than what was
// asked for.

func TestProfileDraft_ProposedSkills(t *testing.T) {
	t.Parallel()

	years := 4.0
	draft := ProfileDraft{Skills: []DraftSkill{
		{Name: "Go", YearsExperience: &years},
		{Name: "  TypeScript  "},
		{Name: "go", YearsExperience: &years},
		{Name: "   "},
	}}

	got := draft.ProposedSkills()

	require.Len(t, got, 2, "a duplicate name and a blank one are dropped")
	assert.Equal(t, "Go", got[0].Name)
	assert.Equal(t, "TypeScript", got[1].Name)
	assert.Nil(t, got[1].YearsExperience, "an unstated duration must not become zero")
}

func TestProfileDraft_ProposedExperiences(t *testing.T) {
	t.Parallel()

	t.Run("reads the dates a resume writes", func(t *testing.T) {
		t.Parallel()
		draft := ProfileDraft{Experiences: []DraftExperience{
			{Company: "Acme", Title: "Staff Engineer", StartDate: "2022-07", IsCurrent: true},
			{Company: "Old Co", Title: "Engineer", StartDate: "2016-09-21", EndDate: "2022-06"},
		}}

		got := draft.ProposedExperiences()

		require.Len(t, got, 2)
		require.NotNil(t, got[0].StartDate)
		assert.Equal(t, 2022, got[0].StartDate.Year())
		assert.Equal(t, time.July, got[0].StartDate.Month())
		assert.Nil(t, got[0].EndDate, "a current role has no end date")
		require.NotNil(t, got[1].EndDate)
	})

	t.Run("an end date of 'Present' means the role is current", func(t *testing.T) {
		t.Parallel()
		// Resumes write this far more often than they set a flag.
		draft := ProfileDraft{Experiences: []DraftExperience{
			{Company: "Acme", Title: "Engineer", StartDate: "2022-07", EndDate: "Present"},
		}}

		got := draft.ProposedExperiences()

		require.Len(t, got, 1)
		assert.True(t, got[0].IsCurrent)
		assert.Nil(t, got[0].EndDate)
	})

	t.Run("an unreadable date is dropped, not guessed", func(t *testing.T) {
		t.Parallel()
		draft := ProfileDraft{Experiences: []DraftExperience{
			{Company: "Acme", Title: "Engineer", StartDate: "sometime in the spring"},
		}}

		got := draft.ProposedExperiences()

		require.Len(t, got, 1, "the role is still worth keeping")
		assert.Nil(t, got[0].StartDate, "it simply does not contribute to a computed span")
	})

	t.Run("an entry naming nothing is dropped", func(t *testing.T) {
		t.Parallel()
		draft := ProfileDraft{Experiences: []DraftExperience{{Description: "Did some things"}}}
		assert.Empty(t, draft.ProposedExperiences())
	})
}

func TestProfileDraft_ProposedLinks(t *testing.T) {
	t.Parallel()

	draft := ProfileDraft{Links: []DraftLink{
		{Kind: "LinkedIn", URL: "https://linkedin.com/in/ada"},
		// A model can return any kind it likes; the profile has one link
		// per kind, so an invented one has nowhere to go.
		{Kind: "facebook", URL: "https://facebook.com/ada"},
		{Kind: "github", URL: ""},
		{Kind: "linkedin", URL: "https://linkedin.com/in/someone-else"},
	}}

	got := draft.ProposedLinks()

	require.Len(t, got, 1)
	assert.Equal(t, LinkKindLinkedIn, got[0].Kind, "the kind is read case-insensitively")
	assert.Equal(t, "https://linkedin.com/in/ada", got[0].URL, "the first of a kind wins")
}

// TestProfileDraft_ProposedLinks_RejectsLabelsThatAreNotURLs covers what a
// real resume produced. Converting the PDF flattened its hyperlinks to
// their label text, so the extraction proposed these in good faith.
func TestProfileDraft_ProposedLinks_RejectsLabelsThatAreNotURLs(t *testing.T) {
	t.Parallel()

	draft := ProfileDraft{Links: []DraftLink{
		{Kind: "github", URL: "ub"},
		{Kind: "linkedin", URL: ""},
		{Kind: "website", URL: "Blog"},
	}}

	assert.Empty(t, draft.ProposedLinks(),
		"a word is not a URL, and one typed into an employer's form is a fact invented about a person")
}

// TestProfileDraft_ProposedLinks_RejectsBareProfileHosts covers the other
// shape the same flattening produced: a real URL that identifies nobody.
func TestProfileDraft_ProposedLinks_RejectsBareProfileHosts(t *testing.T) {
	t.Parallel()

	draft := ProfileDraft{Links: []DraftLink{
		{Kind: "github", URL: "github.com"},
		{Kind: "linkedin", URL: "https://linkedin.com"},
		{Kind: "twitter", URL: "twitter.com/"},
		// A personal site legitimately has no path.
		{Kind: "website", URL: "ada.dev"},
	}}

	got := draft.ProposedLinks()

	require.Len(t, got, 1)
	assert.Equal(t, LinkKindWebsite, got[0].Kind)
	assert.Equal(t, "https://ada.dev", got[0].URL)
}

func TestNormalizeDraftURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{in: "https://github.com/ada", want: "https://github.com/ada", ok: true},
		{in: "http://ada.dev", want: "http://ada.dev", ok: true},
		// People write this and mean it.
		{in: "linkedin.com/in/ada", want: "https://linkedin.com/in/ada", ok: true},
		{in: "www.ada.dev/blog?x=1", want: "https://www.ada.dev/blog?x=1", ok: true},

		// Measured on a real resume whose hyperlinks were flattened.
		{in: "ub", ok: false},
		{in: "Blog", ok: false},
		{in: "LinkedIn", ok: false},
		{in: "", ok: false},
		{in: "   ", ok: false},
		{in: "my portfolio site", ok: false},
		{in: "mailto:ada@example.com", ok: false},
		{in: "javascript:alert(1)", ok: false},
		{in: "https://", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, ok := NormalizeDraftURL(tt.in)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseDraftDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in        string
		wantYear  int
		wantMonth time.Month
		wantNil   bool
	}{
		{in: "2022-07-01", wantYear: 2022, wantMonth: time.July},
		{in: "2022-07", wantYear: 2022, wantMonth: time.July},
		{in: "2022", wantYear: 2022, wantMonth: time.January},
		{in: "July 2022", wantYear: 2022, wantMonth: time.July},
		{in: "Jul 2022", wantYear: 2022, wantMonth: time.July},
		{in: "", wantNil: true},
		{in: "Present", wantNil: true},
		{in: "sometime in 2022", wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got := ParseDraftDate(tt.in)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.wantYear, got.Year())
			assert.Equal(t, tt.wantMonth, got.Month())
		})
	}
}
