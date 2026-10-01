package domain

import (
	"testing"
	"time"

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
		{WorkFrom: "NG"},
	}
	for _, c := range set {
		assert.False(t, c.IsEmpty(), "%+v should count as set", c)
	}
	assert.True(t, SearchCriteria{IncludeRelocation: true}.IsEmpty(),
		"including relocation widens a search; alone it narrows nothing")
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

func TestSearchCriteria_Matches_WorkFrom(t *testing.T) {
	t.Parallel()

	read := func(remote string, locations []string, description string) Job {
		job := Job{RemoteType: remote, Locations: locations, DescriptionMarkdown: description}
		ReadSignals(&job)
		return job
	}
	anywhere := read(RemoteTypeRemote, []string{"Worldwide"}, "")
	emea := read(RemoteTypeRemote, []string{"EMEA"}, "")
	usOnly := read(RemoteTypeRemote, []string{"Remote (US)"}, "")
	unstated := read(RemoteTypeRemote, []string{"Remote"}, "")
	berlinRelocation := read(RemoteTypeOnsite, []string{"Berlin"}, "We offer an attractive relocation package.")
	berlinNoVisa := read(RemoteTypeOnsite, []string{"Berlin"},
		"We offer relocation support. We are unable to offer visa sponsorship for this role.")
	berlinPlain := read(RemoteTypeOnsite, []string{"Berlin"}, "Join our team.")
	unread := Job{RemoteType: RemoteTypeRemote, Locations: []string{"Remote (US)"}}

	fromNigeria := SearchCriteria{WorkFrom: "NG"}
	assert.True(t, fromNigeria.Matches(anywhere))
	assert.True(t, fromNigeria.Matches(emea), "Nigeria is in EMEA")
	assert.False(t, fromNigeria.Matches(usOnly))
	assert.True(t, fromNigeria.Matches(unstated), "a job that does not say is kept, as a job with no salary is")
	assert.True(t, fromNigeria.Matches(unread), "a job not yet read is kept too")
	assert.False(t, fromNigeria.Matches(berlinRelocation), "a job abroad needs the search to include relocation")

	moving := SearchCriteria{WorkFrom: "ng", IncludeRelocation: true}
	assert.True(t, moving.Matches(berlinRelocation))
	assert.False(t, moving.Matches(berlinPlain), "a job abroad that offers no help is left out")
	assert.False(t, moving.Matches(berlinNoVisa),
		"a job that refuses sponsorship is for someone already allowed to work there")
	assert.True(t, moving.MatchesFor(berlinNoVisa, Seeker{AuthorizedCountries: []string{"DE"}}))
	assert.False(t, moving.Matches(usOnly))

	assert.True(t, SearchCriteria{}.Matches(usOnly), "without WorkFrom, nothing changes")
}

func TestCandidate_AuthorizedCountries(t *testing.T) {
	t.Parallel()

	candidate := Candidate{WorkAuthorizations: []WorkAuthorization{
		{Country: "Nigeria", IsAuthorized: true},
		{Country: "Deutschland", IsAuthorized: true},
		{Country: "United States", IsAuthorized: true, RequiresSponsorship: true},
		{Country: "United Kingdom", IsAuthorized: false},
		{Country: "Atlantis", IsAuthorized: true},
		{Country: "nigeria", IsAuthorized: true},
	}}
	assert.Equal(t, []string{"NG", "DE"}, candidate.AuthorizedCountries())
}

func TestSearchTerms(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"backend", "engineer"}, SearchTerms("  Backend   Engineer "))
	assert.Empty(t, SearchTerms(""))
	assert.Empty(t, SearchTerms("   "))
}

func TestSavedSearch_Stale(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	ranAgo := func(d time.Duration) *time.Time {
		at := now.Add(-d)
		return &at
	}

	assert.True(t, (&SavedSearch{}).Stale(now), "a search that never ran is due")
	assert.False(t, (&SavedSearch{LastRunAt: ranAgo(time.Minute)}).Stale(now))
	assert.False(t, (&SavedSearch{LastRunAt: ranAgo(SearchRefreshInterval - time.Second)}).Stale(now))
	assert.True(t, (&SavedSearch{LastRunAt: ranAgo(SearchRefreshInterval)}).Stale(now), "six hours is due, not a second past it")
	assert.True(t, (&SavedSearch{LastRunAt: ranAgo(3 * 24 * time.Hour)}).Stale(now))
}

func TestSearchCriteria_FitHours(t *testing.T) {
	t.Parallel()

	lagos := Seeker{UTCOffset: 1, HasTimeZone: true}
	eastern := Job{Title: "Engineer", Hours: WorkingHours{Status: HoursRequired, From: -9, To: -1}}
	cet := Job{Title: "Engineer", Hours: WorkingHours{Status: HoursRequired, From: -2, To: 4}}
	preferred := Job{Title: "Engineer", Hours: WorkingHours{Status: HoursPreferred, From: -8, To: -8}}
	silent := Job{Title: "Engineer"}

	strict := SearchCriteria{Title: "engineer", FitHours: true}
	assert.True(t, strict.MatchesFor(cet, lagos), "Lagos is inside CET ± 3")
	assert.False(t, strict.MatchesFor(eastern, lagos), "two hours from US Eastern overlap, and no shift")
	assert.True(t, strict.MatchesFor(preferred, lagos), "a preference never leaves a job out (spec D4)")
	assert.True(t, strict.MatchesFor(silent, lagos), "a job that says nothing is kept")
	assert.True(t, strict.MatchesFor(eastern, Seeker{}), "a candidate with no time zone is not filtered")

	shifting := SearchCriteria{Title: "engineer", FitHours: true, HoursShift: 2}
	assert.True(t, shifting.MatchesFor(eastern, lagos), "shifting two hours reaches US Eastern overlap")

	off := SearchCriteria{Title: "engineer"}
	assert.True(t, off.MatchesFor(eastern, lagos), "off until the candidate turns it on")
	assert.True(t, SearchCriteria{FitHours: true}.IsEmpty(), "fitting hours alone narrows almost nothing")
}

func TestCandidate_Seeker(t *testing.T) {
	t.Parallel()

	seeker := Candidate{TimeZone: "Africa/Lagos"}.Seeker()
	assert.True(t, seeker.HasTimeZone)
	assert.Equal(t, 1.0, seeker.UTCOffset)

	india, _ := Candidate{TimeZone: "asia/kolkata"}.UTCOffset()
	assert.Equal(t, 5.5, india)

	_, ok := Candidate{TimeZone: "Mars/Olympus"}.UTCOffset()
	assert.False(t, ok)
}
