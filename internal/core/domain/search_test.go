package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSearchCriteria_IsEmpty(t *testing.T) {
	t.Parallel()

	assert.True(t, SearchCriteria{}.IsEmpty())
	assert.True(t, SearchCriteria{Keywords: "   "}.IsEmpty(), "whitespace is not a criterion")

	set := []SearchCriteria{
		{Keywords: "go"},
		{Title: "engineer"},
		{Location: "lagos"},
		{RemotePreference: "remote"},
		{EmploymentType: "full_time"},
		{SalaryMin: 1},
		{SalaryMax: 1},
		{ExcludedCompanies: []string{"Acme"}},
	}
	for _, c := range set {
		assert.False(t, c.IsEmpty(), "%+v should count as set", c)
	}
}

func TestSearchCriteria_Matches(t *testing.T) {
	t.Parallel()

	job := Job{
		Title:               "Engineer, Backend (Go)",
		CompanyName:         "Acme",
		DescriptionMarkdown: "Work on distributed systems with Kubernetes.",
		Locations:           []string{"Remote", "Lagos"},
		RemoteType:          "remote",
		EmploymentType:      "full_time",
		SalaryMin:           100000,
		SalaryMax:           150000,
	}

	tests := []struct {
		name     string
		criteria SearchCriteria
		want     bool
	}{
		{"empty criteria match anything", SearchCriteria{}, true},

		// The case substring matching got wrong: the words are all present
		// but not in the order, or adjacency, the user typed them.
		{"terms match regardless of order", SearchCriteria{Keywords: "backend engineer"}, true},
		{"terms match reversed", SearchCriteria{Keywords: "engineer backend"}, true},
		{"every term must be present", SearchCriteria{Keywords: "backend rust"}, false},
		{"matching is case insensitive", SearchCriteria{Keywords: "BACKEND"}, true},
		{"keywords search the description too", SearchCriteria{Keywords: "kubernetes"}, true},

		{"title matches the title only", SearchCriteria{Title: "backend"}, true},
		{"title does not match the description", SearchCriteria{Title: "kubernetes"}, false},
		{"location matches any location", SearchCriteria{Location: "lagos"}, true},
		{"location that is absent excludes", SearchCriteria{Location: "berlin"}, false},

		{"remote preference matches exactly", SearchCriteria{RemotePreference: "remote"}, true},
		{"remote preference mismatch excludes", SearchCriteria{RemotePreference: "onsite"}, false},
		{"employment type matches exactly", SearchCriteria{EmploymentType: "full_time"}, true},
		{"employment type mismatch excludes", SearchCriteria{EmploymentType: "contract"}, false},

		{"salary floor within range", SearchCriteria{SalaryMin: 120000}, true},
		{"salary floor above the top end excludes", SearchCriteria{SalaryMin: 200000}, false},
		{"salary ceiling above the bottom end keeps", SearchCriteria{SalaryMax: 200000}, true},
		{"salary ceiling below the bottom end excludes", SearchCriteria{SalaryMax: 50000}, false},

		{"excluded company excludes", SearchCriteria{ExcludedCompanies: []string{"Acme"}}, false},
		{"exclusion is case insensitive", SearchCriteria{ExcludedCompanies: []string{"acme"}}, false},
		{"a different excluded company keeps", SearchCriteria{ExcludedCompanies: []string{"Globex"}}, true},

		{"all criteria must hold together", SearchCriteria{Keywords: "backend", RemotePreference: "onsite"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.criteria.Matches(job))
		})
	}
}

// A salary bound must not discard jobs whose salary is simply unknown:
// Arbeitnow publishes none at all, so treating absence as a mismatch would
// hide that entire source behind any salary preference.
func TestSearchCriteria_Matches_UnknownSalaryIsKept(t *testing.T) {
	t.Parallel()

	job := Job{Title: "Backend Engineer"}

	assert.True(t, SearchCriteria{SalaryMin: 200000}.Matches(job))
	assert.True(t, SearchCriteria{SalaryMax: 10}.Matches(job))
}

func TestSearchTerms(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"backend", "engineer"}, SearchTerms("  Backend   Engineer "))
	assert.Empty(t, SearchTerms(""))
	assert.Empty(t, SearchTerms("   "))
}
