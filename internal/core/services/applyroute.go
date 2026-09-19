package services

import (
	"net/url"
	"strings"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

// maxApplyHops bounds how far from the job posting the worker will travel
// looking for the form (ADR-0013). Two is what a real route costs: the
// posting links to an "apply" page, which redirects to the employer's ATS.
// Anything beyond that is more likely a redirect loop or a tracker.
const maxApplyHops = 2

// applyPhrases are the words an apply control uses, across the languages
// the two current boards publish in. Matching is on whole words so that
// "apply" does not fire on "applying the filters" or "Applied Materials".
var applyPhrases = []string{
	"apply", "apply now", "apply for this job", "application",
	"bewerben", "jetzt bewerben", "postuler", "candidature",
	"i'm interested", "im interested", "submit application",
}

// atsHosts are the applicant tracking systems job boards hand off to. A
// control pointing at one of these is the strongest signal available: it is
// the destination itself rather than a guess about wording.
var atsHosts = []string{
	"ashbyhq.com", "greenhouse.io", "lever.co", "workday.com", "myworkdayjobs.com",
	"smartrecruiters.com", "workable.com", "recruitee.com", "personio.de",
	"bamboohr.com", "teamtailor.com", "jobvite.com", "icims.com",
}

// candidateFieldPatterns are the things an application form asks about a
// person. They are grouped so that several spellings of one question count
// once: a form with "First name" and "Last name" has asked for a name, not
// for two different facts.
var candidateFieldPatterns = map[string][]string{
	"name":          {"name", "vorname", "nachname", "nom", "prénom"},
	"email":         {"email", "e-mail", "courriel"},
	"phone":         {"phone", "telephone", "mobile", "téléphone"},
	"location":      {"location", "city", "country", "where are you based", "ville"},
	"resume":        {"resume", "cv", "lebenslauf", "upload"},
	"coverletter":   {"cover letter", "motivation", "why do you want", "additional information"},
	"links":         {"linkedin", "github", "portfolio", "website"},
	"salary":        {"salary", "compensation", "expectations"},
	"availability":  {"available", "availability", "notice period", "start date"},
	"authorization": {"authorized", "authorised", "sponsorship", "visa", "work permit"},
}

// minCandidateFields is how many distinct things a page must ask about a
// person before it counts as an application form.
//
// Three, because one and two both have common false positives and this is
// the check that stands between the user's details and a stranger's form.
// A newsletter box is a single email field — it was exactly what an earlier
// version filled in with the candidate's address and reported as a prepared
// application — and an email-plus-password pair is a login.
const minCandidateFields = 3

// LooksLikeApplicationForm reports whether a page's fields are asking about
// a person, and is what stands between "we arrived somewhere" and "we
// arrived at the form".
//
// It deliberately ignores whether a <form> element wraps the fields.
// Measured on the route this was built for, that signal is inverted: the
// job board's newsletter box sits inside a <form>, and the employer's
// genuine 23-field application has no <form> element anywhere on the page.
func LooksLikeApplicationForm(fields []ports.FormField) bool {
	return len(matchedCandidateCategories(fields)) >= minCandidateFields
}

func matchedCandidateCategories(fields []ports.FormField) []string {
	seen := map[string]bool{}
	for _, field := range fields {
		label := strings.ToLower(strings.TrimSpace(field.Label))
		if label == "" {
			continue
		}
		for category, patterns := range candidateFieldPatterns {
			if seen[category] {
				continue
			}
			for _, pattern := range patterns {
				if strings.Contains(label, pattern) {
					seen[category] = true
					break
				}
			}
		}
	}

	matched := make([]string, 0, len(seen))
	for category := range seen {
		matched = append(matched, category)
	}
	return matched
}

// PickApplyControl chooses the control most likely to lead to the
// application form, or nil when nothing on the page looks like one.
//
// This is the deterministic half of ADR-0013 and handles the common case
// without a model: a control pointing at a known ATS wins outright, then
// one whose URL path says "apply", then one whose words do. Only when this
// returns nil is the LLM asked, and then only to choose among these same
// controls.
func PickApplyControl(controls []ports.ApplyControl) *ports.ApplyControl {
	best := -1
	bestScore := 0
	for i, control := range controls {
		if !isFollowableControl(control) {
			continue
		}
		if score := scoreApplyControl(control); score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 {
		return nil
	}
	return &controls[best]
}

// isFollowableControl is the deterministic check every destination passes
// before the browser is sent to it, whichever half of ADR-0013 chose it.
//
// A control with no destination is followable: plenty of apply buttons
// navigate by script, and Remotive's does — following one is a click, and
// the click is the point. A javascript: href is the same thing written
// differently. Every other scheme is not a way into a form: mailto: and
// tel: leave the browser entirely, and data:, blob: and file: are a page
// proposing its own content as a destination.
func isFollowableControl(control ports.ApplyControl) bool {
	target := strings.TrimSpace(strings.ToLower(control.URL))
	if target == "" || strings.HasPrefix(target, "javascript:") {
		return true
	}
	return strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://")
}

// followableControls keeps the order and drops what we would refuse to
// follow, so the model in the fallback is offered exactly the controls the
// deterministic rule considered.
func followableControls(controls []ports.ApplyControl) []ports.ApplyControl {
	kept := make([]ports.ApplyControl, 0, len(controls))
	for _, control := range controls {
		if isFollowableControl(control) {
			kept = append(kept, control)
		}
	}
	return kept
}

func scoreApplyControl(control ports.ApplyControl) int {
	target := strings.ToLower(control.URL)
	text := strings.ToLower(strings.TrimSpace(control.Text))

	score := 0
	if host := hostOf(target); host != "" {
		for _, ats := range atsHosts {
			if host == ats || strings.HasSuffix(host, "."+ats) {
				score += 100
				break
			}
		}
	}
	if pathMentionsApply(target) {
		score += 50
	}
	if matchesApplyPhrase(text) {
		score += 25
	}
	return score
}

func hostOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(parsed.Host), "www.")
}

// pathMentionsApply looks only at the path, never the query: a tracking
// parameter that happens to carry the word should not make a link look like
// the way in.
func pathMentionsApply(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	for _, segment := range strings.Split(strings.ToLower(parsed.Path), "/") {
		if segment == "apply" || segment == "application" || segment == "applications" {
			return true
		}
	}
	return false
}

func matchesApplyPhrase(text string) bool {
	if text == "" || len(text) > 60 {
		// A whole paragraph that happens to contain "apply" is prose, not a
		// button.
		return false
	}
	for _, phrase := range applyPhrases {
		if text == phrase {
			return true
		}
		if strings.Contains(text, phrase) && isWholeWordMatch(text, phrase) {
			return true
		}
	}
	return false
}

// isWholeWordMatch guards against "apply" inside "applying" or "reapply".
func isWholeWordMatch(text, phrase string) bool {
	index := strings.Index(text, phrase)
	for index >= 0 {
		beforeOK := index == 0 || !isWordChar(rune(text[index-1]))
		end := index + len(phrase)
		afterOK := end >= len(text) || !isWordChar(rune(text[end]))
		if beforeOK && afterOK {
			return true
		}
		next := strings.Index(text[index+1:], phrase)
		if next < 0 {
			return false
		}
		index += 1 + next
	}
	return false
}

func isWordChar(r rune) bool {
	return r == '_' || r == '\'' ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
