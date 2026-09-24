package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJobPageExtraction_Validate covers the checks ADR-0017 added, which
// did not exist before it: an extracted job was decoded from a whole web
// page and used as-is, with nothing verified but a non-empty title.
func TestJobPageExtraction_Validate(t *testing.T) {
	t.Parallel()

	const url = "https://acme.example/careers/1"

	t.Run("a vocabulary term is matched however it was spelled", func(t *testing.T) {
		t.Parallel()
		tests := map[string]string{
			"Remote": "remote", "REMOTE": "remote", " hybrid ": "hybrid", "Onsite": "onsite",
		}
		for written, want := range tests {
			e := jobPageExtraction{Title: "Engineer", RemoteType: written}
			e.validate(url)
			assert.Equal(t, want, e.RemoteType, "%q", written)
		}

		for written, want := range map[string]string{
			"Full Time": "full-time", "full_time": "full-time", "CONTRACT": "contract",
		} {
			e := jobPageExtraction{Title: "Engineer", EmploymentType: written}
			e.validate(url)
			assert.Equal(t, want, e.EmploymentType, "%q", written)
		}
	})

	t.Run("a term outside the vocabulary is cleared, not kept", func(t *testing.T) {
		t.Parallel()
		// A value nothing can compare is worse than no value: it silently
		// fails every filter instead of being visibly absent.
		e := jobPageExtraction{Title: "Engineer", RemoteType: "work from anywhere",
			EmploymentType: "permanent-ish"}
		e.validate(url)
		assert.Empty(t, e.RemoteType)
		assert.Empty(t, e.EmploymentType)
	})

	t.Run("an unstated salary stays unstated", func(t *testing.T) {
		t.Parallel()
		e := jobPageExtraction{Title: "Engineer", SalaryCurrency: "EUR", SalaryPeriod: "yearly"}
		e.validate(url)
		assert.Zero(t, e.SalaryMin)
		assert.Empty(t, e.SalaryCurrency, "a currency without an amount says nothing")
		assert.Empty(t, e.SalaryPeriod)
	})

	t.Run("a negative salary is discarded", func(t *testing.T) {
		t.Parallel()
		e := jobPageExtraction{Title: "Engineer", SalaryMin: -50000, SalaryMax: 90000,
			SalaryCurrency: "EUR", SalaryPeriod: "yearly"}
		e.validate(url)
		assert.Zero(t, e.SalaryMin)
		assert.Zero(t, e.SalaryMax)
		assert.Empty(t, e.SalaryCurrency)
	})

	t.Run("an inverted range is discarded rather than swapped", func(t *testing.T) {
		t.Parallel()
		// Swapping would be a guess about which number the page meant.
		// The user filters and sorts on this, so a wrong figure is worse
		// than a missing one.
		e := jobPageExtraction{Title: "Engineer", SalaryMin: 120000, SalaryMax: 80000,
			SalaryCurrency: "EUR"}
		e.validate(url)
		assert.Zero(t, e.SalaryMin)
		assert.Zero(t, e.SalaryMax)
	})

	t.Run("a real salary range survives", func(t *testing.T) {
		t.Parallel()
		e := jobPageExtraction{Title: "Engineer", SalaryMin: 80000, SalaryMax: 120000,
			SalaryCurrency: "EUR", SalaryPeriod: "yearly"}
		e.validate(url)
		assert.Equal(t, 80000.0, e.SalaryMin)
		assert.Equal(t, 120000.0, e.SalaryMax)
		assert.Equal(t, "EUR", e.SalaryCurrency)
	})

	t.Run("a company URL that is not a URL is dropped", func(t *testing.T) {
		t.Parallel()
		for _, notAURL := range []string{"acme", "see the website", "javascript:alert(1)", ""} {
			e := jobPageExtraction{Title: "Engineer", CompanyURL: notAURL}
			e.validate(url)
			assert.Empty(t, e.CompanyURL, "%q", notAURL)
		}

		e := jobPageExtraction{Title: "Engineer", CompanyURL: "https://acme.example"}
		e.validate(url)
		assert.Equal(t, "https://acme.example", e.CompanyURL)
	})

	t.Run("blank list entries are dropped", func(t *testing.T) {
		t.Parallel()
		// A model asked for a list returns [""] for "none" often enough
		// that it shows up as an empty location chip in the UI.
		e := jobPageExtraction{Title: "Engineer", Locations: []string{"Berlin", "", "  "},
			Skills: []string{"", "Go"}}
		e.validate(url)
		assert.Equal(t, []string{"Berlin"}, e.Locations)
		assert.Equal(t, []string{"Go"}, e.Skills)
	})
}

func TestParseMatchResponse(t *testing.T) {
	t.Parallel()

	svc := &MatchingService{}

	t.Run("a well-formed reply becomes a match", func(t *testing.T) {
		t.Parallel()
		match, err := svc.parseMatchResponse(`{"score":82,"confidence":0.7,
			"strengths":["Go"],"gaps":[""],"concerns":[],"matched_skills":["Go"],
			"missing_skills":[],"rationale":"  Strong fit.  "}`)

		require.NoError(t, err)
		assert.Equal(t, 82, match.Score)
		assert.Equal(t, 0.7, match.Confidence)
		assert.Equal(t, []string{"Go"}, match.Strengths)
		assert.Nil(t, match.Gaps, "a list of blanks is not a list of gaps")
		assert.Equal(t, "Strong fit.", match.Rationale)
	})

	t.Run("a confidence outside 0..1 is a misread field, not a near miss", func(t *testing.T) {
		t.Parallel()
		_, err := svc.parseMatchResponse(`{"score":80,"confidence":87,"rationale":"x"}`)
		assert.ErrorContains(t, err, "invalid match confidence")
	})

	t.Run("a score outside 0..100 is rejected", func(t *testing.T) {
		t.Parallel()
		_, err := svc.parseMatchResponse(`{"score":5000,"confidence":0.5,"rationale":"x"}`)
		assert.ErrorContains(t, err, "invalid match score")
	})

	t.Run("the model cannot choose the row's identity", func(t *testing.T) {
		t.Parallel()
		// MatchRepository.Save assigns an ID only when one is absent, and
		// this reply is written from an untrusted job posting.
		match, err := svc.parseMatchResponse(
			`{"score":80,"confidence":0.5,"rationale":"x","id":"attacker-chosen","job_id":"other"}`)

		require.NoError(t, err)
		assert.Empty(t, match.ID)
		assert.Empty(t, match.JobID)
		assert.True(t, match.CreatedAt.IsZero())
	})
}

func TestNonEmpty(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"Go", "SQL"}, nonEmpty([]string{" Go ", "", "SQL", "   "}))
	assert.Nil(t, nonEmpty([]string{"", " "}), "nothing left means nothing, not an empty list")
	assert.Nil(t, nonEmpty(nil))
}
