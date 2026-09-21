// Package domain contains the core business entities shared across services,
// independent of any storage, transport, or LLM implementation detail.
package domain

import (
	"strings"
	"time"
)

// LinkKind is what a link is, because forms ask for them one at a time:
// a page with a LinkedIn field and a GitHub field wants a different URL in
// each, and one "links" string could only ever put the same text in both.
type LinkKind string

// Link kinds, drawn from what the recorded forms in
// internal/adapters/secondary/browser/testdata/ats actually ask for.
const (
	LinkKindLinkedIn  LinkKind = "linkedin"
	LinkKindGitHub    LinkKind = "github"
	LinkKindPortfolio LinkKind = "portfolio"
	LinkKindWebsite   LinkKind = "website"
	LinkKindTwitter   LinkKind = "twitter"
	LinkKindOther     LinkKind = "other"
)

// LinkKinds lists every kind, for UI that offers a choice.
func LinkKinds() []LinkKind {
	return []LinkKind{
		LinkKindLinkedIn, LinkKindGitHub, LinkKindPortfolio,
		LinkKindWebsite, LinkKindTwitter, LinkKindOther,
	}
}

// IsValid reports whether a kind is one we have.
func (k LinkKind) IsValid() bool {
	for _, known := range LinkKinds() {
		if k == known {
			return true
		}
	}
	return false
}

// CandidateLink is one of a candidate's professional links.
type CandidateLink struct {
	ID          string    `json:"id"`
	CandidateID string    `json:"candidate_id"`
	Kind        LinkKind  `json:"kind"`
	URL         string    `json:"url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WorkAuthorization is the candidate's right to work in one country.
//
// It is per country because the question always is. "Will you require
// sponsorship?" on a Berlin posting and the same words on a London posting
// are different questions with, for many people, different answers — which
// is exactly what a single prose field could not express.
type WorkAuthorization struct {
	ID          string `json:"id"`
	CandidateID string `json:"candidate_id"`
	// Country is the country's name as the user writes it. There is no
	// country database here: it is matched against a job's stated
	// locations by name, and a question about a country the user has said
	// nothing about is left for them to answer.
	Country             string    `json:"country"`
	IsAuthorized        bool      `json:"is_authorized"`
	RequiresSponsorship bool      `json:"requires_sponsorship"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// Candidate represents a job seeker's profile: identity, preferences, and
// career history used to evaluate and apply to jobs.
//
// The fields a form asks for separately are stored separately (ADR-0015).
// Email and Phone were one Contact string until a real form's phone field
// was filled with an email address; Links was one string until LinkedIn,
// Portfolio and Website each received all of them. What remains as prose
// — Summary, ApplicationPreferences — is read by a person or by an LLM
// writing prose, and structuring it would buy nothing.
type Candidate struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`

	LocationCity    string `json:"location_city"`
	LocationRegion  string `json:"location_region"`
	LocationCountry string `json:"location_country"`

	// NoticePeriodDays and EarliestStartDate answer "when are you
	// available". Both are pointers because zero is an answer —
	// "immediately" — and not the same as unsaid.
	NoticePeriodDays  *int       `json:"notice_period_days,omitempty"`
	EarliestStartDate *time.Time `json:"earliest_start_date,omitempty"`

	Summary                string `json:"summary"`
	WorkPreferences        string `json:"work_preferences"`
	ApplicationPreferences string `json:"application_preferences"`
	Skills                 string `json:"skills"`
	Experiences            string `json:"experiences"`
	Education              string `json:"education"`
	Certifications         string `json:"certifications"`

	// Links and WorkAuthorizations live in their own tables and are loaded
	// with the candidate.
	Links              []CandidateLink     `json:"links"`
	WorkAuthorizations []WorkAuthorization `json:"work_authorizations"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LinkOf returns the candidate's link of a given kind, if they have one.
func (c Candidate) LinkOf(kind LinkKind) (CandidateLink, bool) {
	for _, link := range c.Links {
		if link.Kind == kind {
			return link, true
		}
	}
	return CandidateLink{}, false
}

// LocationDisplay is the candidate's location as a person would write it,
// for prose contexts (cover letters, CLI summaries) that want one string.
func (c Candidate) LocationDisplay() string {
	parts := make([]string, 0, 3)
	for _, part := range []string{c.LocationCity, c.LocationRegion, c.LocationCountry} {
		if strings.TrimSpace(part) != "" {
			parts = append(parts, strings.TrimSpace(part))
		}
	}
	return strings.Join(parts, ", ")
}

// FullName joins the candidate's first and last name for display and
// generated-text contexts (cover letters, CLI summaries) that want a
// single name string.
func (c Candidate) FullName() string {
	return strings.TrimSpace(c.FirstName + " " + c.LastName)
}
