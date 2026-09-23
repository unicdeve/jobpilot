package domain

import (
	"strings"
	"time"
)

// ProfileDraft is an LLM-proposed set of Candidate fields derived from a
// resume, pending user review before being applied to a Candidate profile
// (ADR-0011). Fields the LLM couldn't find in the resume are left as their
// zero value rather than guessed, so callers should only apply non-empty
// fields over whatever the user has already entered.
//
// It does not propose work authorization. A resume does not reliably say
// which countries someone may work in, and a guess there becomes a claim
// about their right to work, made to an employer, under their name
// (ADR-0015 §7).
type ProfileDraft struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`

	LocationCity    string `json:"location_city"`
	LocationRegion  string `json:"location_region"`
	LocationCountry string `json:"location_country"`

	Summary     string            `json:"summary"`
	Skills      []DraftSkill      `json:"skills"`
	Experiences []DraftExperience `json:"experiences"`
	Links       []DraftLink       `json:"links"`
}

// DraftSkill is one proposed skill.
type DraftSkill struct {
	Name string `json:"name"`
	// YearsExperience is a pointer so that a resume which says nothing
	// about duration proposes nothing, rather than proposing zero.
	YearsExperience *float64 `json:"years_experience"`
}

// DraftExperience is one proposed role. Dates arrive as strings because a
// resume writes them a dozen ways and the model should not be asked to
// normalize.
type DraftExperience struct {
	Company     string `json:"company"`
	Title       string `json:"title"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	IsCurrent   bool   `json:"is_current"`
	Description string `json:"description"`
}

// DraftLink is one proposed link.
type DraftLink struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

// Normalize rewrites the draft's dates into the one spelling the rest of
// the system uses, so that what a model read off a resume — "July 2022",
// "03/2019", "Present" — becomes what a date field can hold.
//
// Without this the draft's raw strings travel to the desktop, land in an
// HTML date input that cannot display them, and are rejected when saved.
// The CLI never saw this because it applies ProposedExperiences, which
// parses. Doing it here means both callers get the same dates.
func (d *ProfileDraft) Normalize() {
	for i := range d.Experiences {
		experience := &d.Experiences[i]
		if isPresent(experience.EndDate) {
			// "Present" is a claim that the role is current, and resumes
			// write it far more often than they set a flag.
			experience.IsCurrent = true
		}
		experience.StartDate = normalizedDraftDate(experience.StartDate)
		experience.EndDate = normalizedDraftDate(experience.EndDate)
		if experience.IsCurrent {
			experience.EndDate = ""
		}
	}
}

// normalizedDraftDate returns a date as YYYY-MM-DD, or empty when it could
// not be read. An unreadable date is dropped rather than guessed at.
func normalizedDraftDate(value string) string {
	if parsed := ParseDraftDate(value); parsed != nil {
		return parsed.String()
	}
	return ""
}

// ProposedSkills converts the draft's skills, dropping unnamed ones and
// keeping the first of any duplicate name — the profile holds one entry per
// skill, and a model listing "Go" twice should not become two rows.
func (d ProfileDraft) ProposedSkills() []CandidateSkill {
	seen := map[string]bool{}
	skills := make([]CandidateSkill, 0, len(d.Skills))
	for _, proposed := range d.Skills {
		name := strings.TrimSpace(proposed.Name)
		key := strings.ToLower(name)
		if name == "" || seen[key] {
			continue
		}
		seen[key] = true
		skills = append(skills, CandidateSkill{Name: name, YearsExperience: proposed.YearsExperience})
	}
	return skills
}

// ProposedExperiences converts the draft's work history, dropping entries
// that name neither a company nor a title.
func (d ProfileDraft) ProposedExperiences() []CandidateExperience {
	experiences := make([]CandidateExperience, 0, len(d.Experiences))
	for _, proposed := range d.Experiences {
		company := strings.TrimSpace(proposed.Company)
		title := strings.TrimSpace(proposed.Title)
		if company == "" && title == "" {
			continue
		}

		experience := CandidateExperience{
			Company:     company,
			Title:       title,
			IsCurrent:   proposed.IsCurrent,
			Description: strings.TrimSpace(proposed.Description),
			StartDate:   ParseDraftDate(proposed.StartDate),
			EndDate:     ParseDraftDate(proposed.EndDate),
		}
		// "Present" in an end date is a claim that the role is current, and
		// resumes write it far more often than they set a flag.
		if experience.EndDate == nil && isPresent(proposed.EndDate) {
			experience.IsCurrent = true
		}
		if experience.IsCurrent {
			experience.EndDate = nil
		}
		experiences = append(experiences, experience)
	}
	return experiences
}

// ProposedLinks converts the draft's links, keeping only kinds the profile
// actually has and the first of each.
//
// A model asked for a kind can return anything; "facebook" is not a field
// the profile holds, and a row per invented kind would collide with the one
// link per kind the schema enforces.
func (d ProfileDraft) ProposedLinks() []CandidateLink {
	seen := map[LinkKind]bool{}
	links := make([]CandidateLink, 0, len(d.Links))
	for _, proposed := range d.Links {
		kind := LinkKind(strings.ToLower(strings.TrimSpace(proposed.Kind)))
		url, ok := NormalizeDraftURL(proposed.URL)
		if !ok || !kind.IsValid() || seen[kind] {
			continue
		}
		if identifiesAProfile(kind) && !hasPath(url) {
			// "https://github.com" is the site, not this person. Measured
			// on a real resume: the flattened hyperlinks proposed exactly
			// this, and it would have gone into a form's GitHub field as
			// the candidate's profile.
			continue
		}
		seen[kind] = true
		links = append(links, CandidateLink{Kind: kind, URL: url})
	}
	return links
}

// identifiesAProfile reports whether a link kind names a page belonging to
// one person, as opposed to somewhere they publish. A personal site is
// legitimately a bare host; a LinkedIn profile never is.
func identifiesAProfile(kind LinkKind) bool {
	switch kind {
	case LinkKindLinkedIn, LinkKindGitHub, LinkKindTwitter:
		return true
	default:
		return false
	}
}

// hasPath reports whether a URL points at something under its host.
func hasPath(url string) bool {
	rest := url
	if cut := strings.Index(rest, "://"); cut >= 0 {
		rest = rest[cut+3:]
	}
	slash := strings.Index(rest, "/")
	return slash >= 0 && strings.Trim(rest[slash:], "/") != ""
}

// NormalizeDraftURL returns a usable URL, or false when the value is not
// one at all.
//
// This is not defensive programming for its own sake. Measured on a real
// resume: converting the PDF flattened its hyperlinks to their label text,
// so the extraction faithfully proposed "ub" as a GitHub URL, "Blog" as a
// website and an empty string for LinkedIn. Without this, those become the
// candidate's links and get typed into an employer's form — which is the
// exact defect ADR-0015 exists to prevent, arriving by a different door.
//
// A bare host is accepted and given a scheme, because people write
// "linkedin.com/in/ada" and mean it. A word is not a URL however confident
// the model was.
func NormalizeDraftURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\n\r") {
		return "", false
	}

	rest := raw
	hadScheme := false
	for _, scheme := range []string{"https://", "http://"} {
		if len(rest) >= len(scheme) && strings.EqualFold(rest[:len(scheme)], scheme) {
			rest = rest[len(scheme):]
			hadScheme = true
			break
		}
	}
	if !hadScheme && hasScheme(raw) {
		// Some other scheme entirely. mailto: and javascript: are not
		// links to a profile, and mailto: in particular would otherwise
		// pass as a host, since an email address has a dot in it.
		return "", false
	}

	host := rest
	if cut := strings.IndexAny(host, "/?#"); cut >= 0 {
		host = host[:cut]
	}
	host = strings.TrimSuffix(host, ".")
	if !strings.Contains(host, ".") {
		return "", false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return "", false
		}
	}

	if hadScheme {
		return raw, true
	}
	return "https://" + raw, true
}

// hasScheme reports whether a value opens with a URL scheme — "mailto:",
// "javascript:", "file:". A host with a port ("example.com:8080") is not
// one, because a scheme never contains a dot.
func hasScheme(raw string) bool {
	for i, r := range raw {
		switch {
		case r == ':':
			return i > 0
		case r == '.' || r == '/':
			return false
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9', r == '+', r == '-':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return false
}

// ParseDraftDate reads a date from a resume, or returns nil when it cannot.
//
// Nil is a normal outcome, not a failure: an entry whose dates could not be
// read is still worth keeping, it simply does not contribute to a computed
// span. A date guessed from an unreadable string would.
func ParseDraftDate(value string) *Date {
	value = strings.TrimSpace(value)
	if value == "" || isPresent(value) {
		return nil
	}
	if parsed, ok := ParseDate(value); ok {
		return &parsed
	}
	// The layouts above are the ones the API speaks. A resume is not an
	// API, and writes "Jan 2022" or "03/2019" as readily as an ISO date,
	// so those are read here rather than widened into the wire format.
	for _, layout := range resumeDateLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			date := DateOf(parsed)
			return &date
		}
	}
	return nil
}

// resumeDateLayouts are the human spellings a resume uses.
var resumeDateLayouts = []string{
	"January 2006", "Jan 2006", "01/2006", "2006/01",
	"January 2, 2006", "Jan 2, 2006", "02/01/2006",
}

func isPresent(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "present", "current", "now", "ongoing", "to date", "presently":
		return true
	default:
		return false
	}
}
