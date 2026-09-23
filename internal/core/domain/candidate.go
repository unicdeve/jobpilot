// Package domain contains the core business entities shared across services,
// independent of any storage, transport, or LLM implementation detail.
package domain

import (
	"fmt"
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

// RemotePreference is how the candidate wants to work, in the same
// vocabulary Job.RemoteType uses.
//
// Education and certifications used to sit beside this as prose. They were
// removed in ADR-0015's closing change: nothing read them and no interface
// ever wrote them, so they held no data for a consumer that did not exist.
type RemotePreference string

// Remote preferences. The empty value means the candidate has not said.
const (
	RemotePreferenceRemote RemotePreference = "remote"
	RemotePreferenceHybrid RemotePreference = "hybrid"
	RemotePreferenceOnsite RemotePreference = "onsite"
)

// RemotePreferences lists every value, for UI that offers a choice.
func RemotePreferences() []RemotePreference {
	return []RemotePreference{RemotePreferenceRemote, RemotePreferenceHybrid, RemotePreferenceOnsite}
}

// IsValid reports whether a preference is one we have. The empty value is
// valid: not having said is a normal state, and different from every
// answer.
func (p RemotePreference) IsValid() bool {
	if p == "" {
		return true
	}
	for _, known := range RemotePreferences() {
		if p == known {
			return true
		}
	}
	return false
}

// CandidateExperience is one job the candidate has held.
//
// Dates rather than prose is the whole point: a paragraph cannot say which
// employer is the current one, and cannot be counted.
type CandidateExperience struct {
	ID          string `json:"id"`
	CandidateID string `json:"candidate_id"`
	Company     string `json:"company"`
	Title       string `json:"title"`
	// StartDate and EndDate are optional, because a half-remembered
	// history is still worth storing. An entry without a start date simply
	// does not contribute to a computed span.
	StartDate *Date `json:"start_date,omitempty"`
	EndDate   *Date `json:"end_date,omitempty"`
	IsCurrent bool  `json:"is_current"`
	// Description stays prose on purpose: it is read by a person, or by an
	// LLM writing a cover letter, and structuring it would buy nothing.
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CandidateSkill is one thing the candidate can do.
type CandidateSkill struct {
	ID          string `json:"id"`
	CandidateID string `json:"candidate_id"`
	Name        string `json:"name"`
	// YearsExperience is optional: "I know Go" and "I have used Go for
	// four years" are different statements, and only the candidate can
	// make the second.
	YearsExperience *float64  `json:"years_experience,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
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
	NoticePeriodDays  *int  `json:"notice_period_days,omitempty"`
	EarliestStartDate *Date `json:"earliest_start_date,omitempty"`

	Summary string `json:"summary"`
	// RemotePreference uses the same vocabulary as Job.RemoteType, so a
	// preference can be compared with a posting rather than searched for a
	// word inside a paragraph.
	RemotePreference       RemotePreference `json:"remote_preference"`
	ApplicationPreferences string           `json:"application_preferences"`

	// These live in their own tables and are loaded with the candidate.
	Links              []CandidateLink       `json:"links"`
	WorkAuthorizations []WorkAuthorization   `json:"work_authorizations"`
	Skills             []CandidateSkill      `json:"skills"`
	Experiences        []CandidateExperience `json:"experiences"`

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

// CurrentExperience returns the job the candidate holds now, which is what
// a form means by "Current company".
//
// The flag wins over the dates: a candidate who has said which job is
// current has answered the question directly. Failing that, the entry with
// no end date and the latest start is the best available reading.
func (c Candidate) CurrentExperience() (CandidateExperience, bool) {
	for _, experience := range c.Experiences {
		if experience.IsCurrent {
			return experience, true
		}
	}

	var best CandidateExperience
	found := false
	for _, experience := range c.Experiences {
		if experience.EndDate != nil || experience.StartDate == nil {
			continue
		}
		if !found || experience.StartDate.After(*best.StartDate) {
			best, found = experience, true
		}
	}
	return best, found
}

// TotalYearsOfExperience is how long the candidate has been working,
// measured from the earliest start date to the latest end — or to now,
// while a job is still current.
//
// It measures the span rather than summing the entries, because two jobs
// held at once are not twice the experience. It returns false when no entry
// carries a start date, since a number invented from nothing is exactly
// what principle 6 forbids.
func (c Candidate) TotalYearsOfExperience(now time.Time) (float64, bool) {
	var earliest, latest time.Time
	for _, experience := range c.Experiences {
		if experience.StartDate == nil {
			continue
		}
		start := experience.StartDate.Time()
		if earliest.IsZero() || start.Before(earliest) {
			earliest = start
		}

		end := now
		if !experience.IsCurrent && experience.EndDate != nil {
			end = experience.EndDate.Time()
		}
		if latest.IsZero() || end.After(latest) {
			latest = end
		}
	}
	if earliest.IsZero() || !latest.After(earliest) {
		return 0, false
	}

	return latest.Sub(earliest).Hours() / (24 * 365.25), true
}

// SkillNamed finds a skill by name, case-insensitively.
func (c Candidate) SkillNamed(name string) (CandidateSkill, bool) {
	for _, skill := range c.Skills {
		if strings.EqualFold(strings.TrimSpace(skill.Name), strings.TrimSpace(name)) {
			return skill, true
		}
	}
	return CandidateSkill{}, false
}

// SkillNames lists the candidate's skills, for matching against a job's own
// skill list and for prompts that want them as text.
func (c Candidate) SkillNames() []string {
	names := make([]string, 0, len(c.Skills))
	for _, skill := range c.Skills {
		if name := strings.TrimSpace(skill.Name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// RenderSkills is the candidate's skills as one line, for a prompt.
func (c Candidate) RenderSkills() string {
	return strings.Join(c.SkillNames(), ", ")
}

// RenderExperiences is the candidate's work history as text, for a prompt.
//
// The model reading this wants prose, so the structure is rendered back
// into it. That is not a round trip to nowhere: the rendering is
// consistent, dated and ordered, where the blob it replaces was whatever
// the user or an extraction happened to type.
func (c Candidate) RenderExperiences() string {
	var sb strings.Builder
	for _, experience := range c.Experiences {
		fmt.Fprintf(&sb, "- %s", strings.TrimSpace(experience.Title))
		if experience.Company != "" {
			fmt.Fprintf(&sb, " at %s", strings.TrimSpace(experience.Company))
		}
		if period := experience.Period(); period != "" {
			fmt.Fprintf(&sb, " (%s)", period)
		}
		sb.WriteString("\n")
		if description := strings.TrimSpace(experience.Description); description != "" {
			fmt.Fprintf(&sb, "  %s\n", description)
		}
	}
	return strings.TrimSpace(sb.String())
}

// Period renders an experience's dates the way a CV does.
func (e CandidateExperience) Period() string {
	switch {
	case e.StartDate == nil && e.EndDate == nil:
		return ""
	case e.StartDate == nil:
		return "until " + e.EndDate.Format("Jan 2006")
	case e.IsCurrent || e.EndDate == nil:
		return e.StartDate.Format("Jan 2006") + " – present"
	default:
		return e.StartDate.Format("Jan 2006") + " – " + e.EndDate.Format("Jan 2006")
	}
}
