package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// This file tests the resolver against the question labels of real
// application forms, recorded in
// internal/adapters/secondary/browser/testdata/ats.
//
// Every case here was a measured defect. Running the previous resolver over
// these same labels put the candidate's email address in both forms' phone
// fields, put every one of their URLs in each of LinkedIn, Portfolio and
// Website while leaving GitHub and Twitter empty, and answered a Yes/No
// sponsorship question with a paragraph of prose. That is what ADR-0015 was
// written about, and this is what stops it coming back.

// testCandidate is a filled-in profile in the shape ADR-0015 gives it.
func testCandidate() *domain.Candidate {
	notice := 30
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	return &domain.Candidate{
		FirstName:        "Ada",
		LastName:         "Lovelace",
		Email:            "ada@example.com",
		Phone:            "+49 30 123456",
		LocationCity:     "Berlin",
		LocationCountry:  "Germany",
		NoticePeriodDays: &notice,
		// A date and a notice period together: the date is the better
		// answer to "when are you available", so it wins.
		EarliestStartDate: &start,
		WorkPreferences:   "Remote or hybrid in Berlin.",
		Links: []domain.CandidateLink{
			{Kind: domain.LinkKindLinkedIn, URL: "https://linkedin.com/in/ada"},
			{Kind: domain.LinkKindGitHub, URL: "https://github.com/ada"},
			{Kind: domain.LinkKindWebsite, URL: "https://ada.dev"},
		},
		WorkAuthorizations: []domain.WorkAuthorization{
			{Country: "Germany", IsAuthorized: true, RequiresSponsorship: false},
			{Country: "United Kingdom", IsAuthorized: false, RequiresSponsorship: true},
		},
		Skills: []domain.CandidateSkill{
			{Name: "Go", YearsExperience: ptr(4.0)},
			{Name: "React Native", YearsExperience: ptr(2.0)},
			// Deliberately no duration: the candidate has the skill but
			// has not said for how long.
			{Name: "TypeScript"},
		},
		Experiences: []domain.CandidateExperience{
			{
				Company: "Acme", Title: "Staff Engineer",
				StartDate: date(2022, time.July, 1), IsCurrent: true,
			},
			{
				Company: "Old Co", Title: "Engineer",
				StartDate: date(2016, time.September, 21), EndDate: date(2022, time.June, 30),
			},
		},
	}
}

func ptr[T any](v T) *T { return &v }

func date(year int, month time.Month, day int) *time.Time {
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return &t
}

func germanJob() *domain.Job {
	return &domain.Job{Locations: []string{"Berlin, Germany"}}
}

// TestProfileFactAnswer_OnRecordedFormLabels states, label by label, what a
// real form gets from a filled-in profile.
func TestProfileFactAnswer_OnRecordedFormLabels(t *testing.T) {
	t.Parallel()

	candidate := testCandidate()

	tests := []struct {
		name  string
		label string
		want  string
	}{
		// The defect the whole change started from. Both of these are real
		// labels from the recorded Ashby and Greenhouse forms, and both
		// used to return the email address.
		{"email asks for the email", "Email", "ada@example.com"},
		{"phone asks for the phone", "Phone", "+49 30 123456"},
		{"phone with a required marker stripped", "Phone ", "+49 30 123456"},

		// Each link field gets its own link, and a field we have no link
		// for gets nothing rather than somebody else's URL.
		{"linkedin", "LinkedIn Profile", "https://linkedin.com/in/ada"},
		{"github", "Github", "https://github.com/ada"},
		{"website", "Website", "https://ada.dev"},
		{"twitter is not stored, so it is not answered", "Twitter", ""},
		{"portfolio is not stored, so it is not answered", "Portfolio", ""},

		{"first name", "First Name", "Ada"},
		{"last name", "Last Name", "Lovelace"},
		{"a single name field gets the full name", "Name", "Ada Lovelace"},

		// Greenhouse asks for Country and City as separate fields; Ashby
		// asks for one location.
		{"country", "Country", "Germany"},
		{"city", "Location (City)", "Berlin"},
		{"a plain location field gets the whole location", "Current location", "Berlin, Germany"},
		{"where are you based reads as a location, not a decision", "Where are you based?", "Berlin, Germany"},

		{"availability prefers the stated date", "When are you available", "2026-11-01"},

		// Asked by the recorded Ashby and Lever forms, and unanswerable
		// until the work history had dates (ADR-0015 stage two).
		{"current company", "Current company", "Acme"},
		{"current title", "Current title", "Staff Engineer"},

		// Salary has no profile field at all now: the filter lives on the
		// saved search, and telling an employer a search floor would state
		// a number the candidate never gave (ADR-0015 §4).
		{"salary is left for the user", "What are your salary expectations annually ?", ""},

		// Decision-shaped labels are not fact lookups. Each of these was
		// answered wrongly before: the countries question got the
		// candidate's city, because the old matcher saw "based".
		{
			"a country list question is not a location field",
			"Are you currently based in any of these countries? Please note these are the only countries where we are accepting applications",
			"",
		},
		{"an office-attendance question is not a fact", "Are you able to work from our London office on a hybrid schedule, 3 days a week?", ""},
		{"a states question is not a region field", "Do you live in one of the following states? Alabama, Alaska, Delaware", ""},
		{"a self-identification question is never answered", "Do you identify as transgender?", ""},
		{"a veteran question is not a region field despite 'States'", "Are you a veteran or active member of the United States Armed Forces?", ""},
		{"an acknowledgement is not a fact", "By submitting my application, I acknowledge that I have read and understand the privacy notice", ""},

		// Not every "name" on a form is the candidate's.
		// This used to return "Ada Lovelace", which is what the case was
		// written to catch. It now returns the company, which is what the
		// question actually asks for — so the expectation moves with it.
		{"a company name field gets the company, never the candidate's name", "Current company name", "Acme"},
		{"a school name field is not the candidate's name", "Name of university", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, got := profileFactAnswer(candidate, tt.label)
			assert.Equal(t, tt.want, got, "label %q", tt.label)
		})
	}
}

func TestProfileFactAnswer_EmptyProfileAnswersNothing(t *testing.T) {
	t.Parallel()

	// A field we have no value for must fall through to the user rather
	// than filling a form with an empty string.
	empty := &domain.Candidate{}
	for _, label := range []string{"Email", "Phone", "First Name", "LinkedIn", "Country", "Current location"} {
		_, got := profileFactAnswer(empty, label)
		assert.Empty(t, got, "label %q", label)
	}
}

func TestWorkAuthorizationAnswer(t *testing.T) {
	t.Parallel()

	candidate := testCandidate()

	t.Run("answers sponsorship for the country the job is in", func(t *testing.T) {
		t.Parallel()
		_, got := workAuthorizationAnswer(candidate, germanJob(),
			"Will you require Visa Sponsorship now, or in the future?")
		assert.Equal(t, "No", got, "the candidate is authorized in Germany and needs no sponsorship")
	})

	t.Run("the same question about another country gets the other answer", func(t *testing.T) {
		t.Parallel()
		london := &domain.Job{Locations: []string{"London, United Kingdom"}}

		_, got := workAuthorizationAnswer(candidate, london,
			"Will you require Visa Sponsorship now, or in the future?")

		assert.Equal(t, "Yes", got,
			"this is why authorization is per country: one prose field could not say both")
	})

	t.Run("answers a right-to-work question", func(t *testing.T) {
		t.Parallel()
		_, got := workAuthorizationAnswer(candidate, germanJob(), "Are you authorized to work in this country?")
		assert.Equal(t, "Yes", got)
	})

	t.Run("says nothing about a country the candidate has not spoken about", func(t *testing.T) {
		t.Parallel()
		elsewhere := &domain.Job{Locations: []string{"Toronto, Canada"}}

		_, got := workAuthorizationAnswer(candidate, elsewhere,
			"Will you require Visa Sponsorship now, or in the future?")

		assert.Empty(t, got,
			"a claim about someone's right to work, made to an employer, must not be guessed")
	})

	t.Run("says nothing when the job has no location", func(t *testing.T) {
		t.Parallel()
		_, got := workAuthorizationAnswer(candidate, &domain.Job{}, "Do you require sponsorship?")
		assert.Empty(t, got)
	})

	t.Run("says nothing when there is no job at all", func(t *testing.T) {
		t.Parallel()
		_, got := workAuthorizationAnswer(candidate, nil, "Do you require sponsorship?")
		assert.Empty(t, got)
	})

	t.Run("leaves questions that are not about working rights alone", func(t *testing.T) {
		t.Parallel()
		for _, label := range []string{"Email", "How did you hear about us?", "Do you identify as transgender?"} {
			_, got := workAuthorizationAnswer(candidate, germanJob(), label)
			assert.Empty(t, got, "label %q", label)
		}
	})
}

// TestEveryRecordedFormLabelIsAnsweredOrRefused walks the actual recorded
// forms and asserts that nothing is answered with a value belonging to a
// different question.
//
// The per-label expectations above are the specification; this is the net
// that catches a label nobody thought to write a case for, on a page we did
// not write.
func TestEveryRecordedFormLabelIsAnsweredOrRefused(t *testing.T) {
	t.Parallel()

	candidate := testCandidate()

	// What must never appear as the answer to a question it does not
	// belong to. Each of these was a real wrong answer.
	forbidden := map[string][]string{
		"Email":   {candidate.Phone},
		"Phone":   {candidate.Email},
		"Twitter": {"https://linkedin.com/in/ada", "https://github.com/ada", "https://ada.dev"},
		"Github":  {"https://linkedin.com/in/ada", "https://ada.dev"},
		"Website": {"https://linkedin.com/in/ada", "https://github.com/ada"},
	}

	for _, fixture := range []string{"ashby-application", "greenhouse-application", "lever-application"} {
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			for _, label := range recordedFormLabels(t, fixture) {
				_, answer := profileFactAnswer(candidate, label)
				if answer == "" {
					continue
				}
				for _, banned := range forbidden[label] {
					assert.NotEqual(t, banned, answer,
						"%q was answered with a value belonging to another question", label)
				}
				assert.NotContains(t, answer, " https://",
					"%q got more than one URL, which is the links-blob defect returning", label)
			}
		})
	}
}

func recordedFormLabels(t *testing.T, fixture string) []string {
	t.Helper()

	path := filepath.Join("..", "..", "adapters", "secondary", "browser", "testdata", "ats", fixture+".golden.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "recorded form missing; see that directory's README")

	var golden struct {
		Fields []struct {
			Label string `json:"label"`
		} `json:"fields"`
	}
	require.NoError(t, json.Unmarshal(raw, &golden))

	labels := make([]string, 0, len(golden.Fields))
	for _, field := range golden.Fields {
		labels = append(labels, field.Label)
	}
	require.NotEmpty(t, labels)
	return labels
}

func TestYearsOfExperienceAnswer(t *testing.T) {
	t.Parallel()

	candidate := testCandidate()
	now := time.Date(2026, time.September, 21, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		label string
		want  string
	}{
		{"a named skill is answered from what was said about it", "How many years of experience do you have with Go?", "4"},
		{"the longest matching skill wins", "Years of experience with React Native", "2"},
		{"a whole career when no skill is named", "How many years of experience do you have?", "10"},

		// The one that matters: the candidate has never said how long they
		// have used TypeScript, and their total career is not an answer to
		// a question about TypeScript.
		{"a skill with no stated duration is not answered from the career total", "How many years of experience do you have with TypeScript?", ""},
		{"a skill they do not have is not answered at all", "How many years of experience do you have with Rust?", ""},

		{"not a duration question", "What is your current company?", ""},
		// "Go" must not match "Google", and the career total must not stand
		// in for a skill the candidate never claimed.
		{"a word that merely contains a skill name is not that skill", "How many years of experience do you have with Google Cloud?", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, got := yearsOfExperienceAnswer(candidate, normalizeLabel(tt.label), now)
			assert.Equal(t, tt.want, got, "label %q", tt.label)
		})
	}

	t.Run("a candidate with no dated history says nothing", func(t *testing.T) {
		t.Parallel()
		bare := &domain.Candidate{Experiences: []domain.CandidateExperience{{Company: "Acme"}}}

		_, got := yearsOfExperienceAnswer(bare, "how many years of experience do you have?", now)

		assert.Empty(t, got)
	})
}
