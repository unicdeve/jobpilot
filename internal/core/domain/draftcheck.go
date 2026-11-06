package domain

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// DraftFlagKind is what a flag on a letter or written answer is about
// (docs/02_features/19_draft_checks.md).
type DraftFlagKind string

// The kinds of flag a draft can carry.
const (
	FlagStockPhrase      DraftFlagKind = "stock_phrase"
	FlagLength           DraftFlagKind = "length"
	FlagUnsupportedClaim DraftFlagKind = "unsupported_claim"
)

// DraftFlag is one thing in a draft the candidate may want to look at
// before they send it. It is advice, never a gate.
type DraftFlag struct {
	Kind DraftFlagKind `json:"kind"`
	// Excerpt is the flagged words as the draft has them, for the
	// candidate to find; a length flag has none.
	Excerpt string `json:"excerpt,omitempty"`
	Reason  string `json:"reason"`
}

// LetterWords is the length a letter is held to (D2).
const LetterWords = 250

// DraftBasis is what a draft is checked against: the candidate's own facts
// and words, never the posting's.
type DraftBasis struct {
	// WordLimit is the most words the draft may have. Zero checks no
	// length, which is how a written answer is checked (D2).
	WordLimit int

	Skills []CandidateSkill
	// CareerYears is the span of the candidate's dated work history, when
	// HasCareer says there is one.
	CareerYears float64
	HasCareer   bool
	// OwnWords is everything the candidate wrote or gave: resume, profile,
	// what they said about themselves. A claim they made there is theirs.
	OwnWords []string
}

// DraftBasisFor gathers a candidate's facts into a basis, with their
// profile's prose and any further text of their own, such as the resume.
// It sets no word limit; a letter sets LetterWords.
func DraftBasisFor(candidate Candidate, now time.Time, ownWords ...string) DraftBasis {
	basis := DraftBasis{Skills: candidate.Skills, OwnWords: []string{candidate.Summary}}
	basis.CareerYears, basis.HasCareer = candidate.TotalYearsOfExperience(now)
	for _, experience := range candidate.Experiences {
		basis.OwnWords = append(basis.OwnWords, experience.Description)
	}
	basis.OwnWords = append(basis.OwnWords, ownWords...)
	return basis
}

// CheckDraft returns what the candidate may want to look at in a letter or
// written answer: stock phrases, a length past the limit, and claims of
// years the basis does not support. It involves no model; the same text and
// basis always give the same flags.
func CheckDraft(text string, basis DraftBasis) []DraftFlag {
	flags := make([]DraftFlag, 0)
	if basis.WordLimit > 0 {
		if words := len(strings.Fields(text)); words > basis.WordLimit {
			flags = append(flags, DraftFlag{
				Kind:   FlagLength,
				Reason: fmt.Sprintf("%d words, over the %d a letter reads best within.", words, basis.WordLimit),
			})
		}
	}

	seen := map[string]bool{}
	add := func(flag DraftFlag) {
		key := string(flag.Kind) + "\x00" + strings.ToLower(flag.Excerpt) + "\x00" + flag.Reason
		if !seen[key] {
			seen[key] = true
			flags = append(flags, flag)
		}
	}

	skills := skillMatchers(basis.Skills)
	ownClaims := claimsIn(basis.OwnWords, skills)
	for _, sentence := range Sentences(text) {
		for _, phrase := range stockPhrases {
			for _, excerpt := range phrase.FindAllString(sentence, -1) {
				add(DraftFlag{
					Kind:    FlagStockPhrase,
					Excerpt: excerpt,
					Reason:  "A stock phrase: any applicant could have written it.",
				})
			}
		}
		for _, claim := range sentenceClaims(sentence, skills) {
			if reason, ok := unsupported(claim, basis, ownClaims); ok {
				add(DraftFlag{Kind: FlagUnsupportedClaim, Excerpt: claim.excerpt, Reason: reason})
			}
		}
	}
	return flags
}

// iAm and its kind match a phrase whether or not it is contracted.
const (
	iAm    = `i(?:\s+am|['’]m)`
	iWould = `i(?:\s+would|['’]d)`
)

// stockPhrases are the openings, closings and fillers that make a letter
// read as anyone's, and as written by a model: the ones measured in letters
// llama3.2:1b wrote while JobPilot still drafted them
// (docs/02_features/16_about_you.md).
var stockPhrases = func() []*regexp.Regexp {
	sources := []string{
		iAm + ` (?:so |very |truly )?(?:excited|thrilled|delighted|eager) to apply`,
		iAm + ` (?:so |very |truly )?(?:excited|thrilled|delighted) (?:about|by) the opportunity`,
		iAm + ` writing to (?:express my (?:strong |keen )?interest|apply)`,
		`(?:a|an|the) (?:great|perfect|ideal|strong|excellent) fit`,
		`thank you for (?:your (?:time and )?consideration|considering my application|your time)`,
		`i look forward to hearing from you`,
		iWould + ` welcome the (?:opportunity|chance)`,
		iAm + ` confident (?:that )?i`,
		`hit the ground running`,
		`team player`,
		`proven track record`,
		`to whom it may concern`,
		`(?:a )?valuable (?:asset|addition) to`,
		`unique blend of`,
		`aligns? perfectly with`,
		iAm + ` passionate about`,
	}
	phrases := make([]*regexp.Regexp, len(sources))
	for i, source := range sources {
		source = strings.ReplaceAll(source, " ", `\s+`)
		phrases[i] = regexp.MustCompile(`(?i)\b` + source + `\b`)
	}
	return phrases
}()

// claim is a number of years a sentence says the candidate has.
type claim struct {
	years   float64
	excerpt string
	skills  []CandidateSkill
}

// yearWords are the numbers of years a draft may write out.
var yearWords = map[string]float64{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7,
	"eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13,
	"fourteen": 14, "fifteen": 15, "sixteen": 16, "seventeen": 17, "eighteen": 18,
	"nineteen": 19, "twenty": 20,
}

// yearsPhrase finds a number of years: "8 years", "8+ years", "over eight
// years", "five-plus years", "a 3-year stint". Its last group is what
// makes it an age or a time ago rather than a claim.
var yearsPhrase = regexp.MustCompile(`(?i)\b((?:over|more than|nearly|almost|about|around)\s+)?` +
	`(\d{1,2}(?:\.\d+)?|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen|twenty)` +
	`(?:\s*\+|-plus|\s+plus)?[\s-]+years?\b(-old|\s+old|\s+ago)?`)

// claimSentence is a sentence that speaks for the candidate or of
// experience; a number of years elsewhere is about something else.
var claimSentence = regexp.MustCompile(`(?i)\b(?:i|i['’](?:m|ve|d)|my|me|experience[sd]?)\b`)

// futureWords before a number of years make it a time to come: "in the
// next five years".
var futureWords = map[string]bool{"next": true, "coming": true, "within": true}

// claimsIn reads the claims in the candidate's own words. Every sentence
// there is theirs, so none needs to speak in the first person: "Ran
// Kubernetes in production for 3 years" is a resume line.
func claimsIn(texts []string, skills []skillMatcher) []claim {
	var claims []claim
	for _, text := range texts {
		for _, sentence := range Sentences(text) {
			claims = append(claims, yearsIn(sentence, skills)...)
		}
	}
	return claims
}

// sentenceClaims reads the numbers of years a sentence of a draft claims
// for the candidate, each with the skills the sentence names.
func sentenceClaims(sentence string, skills []skillMatcher) []claim {
	if !claimSentence.MatchString(sentence) {
		return nil
	}
	return yearsIn(sentence, skills)
}

// yearsIn reads every number of years in a sentence that is neither an age,
// a time ago nor a time to come.
func yearsIn(sentence string, skills []skillMatcher) []claim {
	var claims []claim
	for _, m := range yearsPhrase.FindAllStringSubmatchIndex(sentence, -1) {
		if m[6] >= 0 {
			continue // an age, or a time ago
		}
		if words := strings.Fields(sentence[:m[0]]); len(words) > 0 && futureWords[strings.ToLower(words[len(words)-1])] {
			continue
		}
		number := strings.ToLower(sentence[m[4]:m[5]])
		years, ok := yearWords[number]
		if !ok {
			var err error
			if years, err = strconv.ParseFloat(number, 64); err != nil {
				continue
			}
		}
		claims = append(claims, claim{
			years:   years,
			excerpt: sentence[m[0]:m[1]],
			skills:  skillsNamed(sentence, skills),
		})
	}
	return claims
}

// skillMatcher finds one of the candidate's skills in a sentence.
type skillMatcher struct {
	skill   CandidateSkill
	pattern *regexp.Regexp
}

// skillMatchers compiles a pattern for each of the candidate's skills,
// once per check. A name of three letters or fewer must match its case, so
// the skill Go is not found in "helping teams go faster".
func skillMatchers(skills []CandidateSkill) []skillMatcher {
	matchers := make([]skillMatcher, 0, len(skills))
	for _, skill := range skills {
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			continue
		}
		pattern := `(?:^|[^\w+#.])` + regexp.QuoteMeta(name) + `(?:$|[^\w+#])`
		if len([]rune(name)) > 3 {
			pattern = `(?i)` + pattern
		}
		matchers = append(matchers, skillMatcher{skill: skill, pattern: regexp.MustCompile(pattern)})
	}
	return matchers
}

// skillsNamed is the candidate's skills a sentence names.
func skillsNamed(sentence string, skills []skillMatcher) []CandidateSkill {
	var named []CandidateSkill
	for _, matcher := range skills {
		if matcher.pattern.MatchString(sentence) {
			named = append(named, matcher.skill)
		}
	}
	return named
}

// unsupported says why a claim is not supported by the basis, or false
// when it is (D1).
func unsupported(c claim, basis DraftBasis, own []claim) (string, bool) {
	if saidThemselves(c, own) {
		return "", false
	}
	if len(c.skills) > 0 {
		for _, skill := range c.skills {
			if skill.YearsExperience == nil {
				return fmt.Sprintf("Claims %s of %s, and you have not said how long you have used it.", yearsText(c.years), skill.Name), true
			}
			if c.years > math.Round(*skill.YearsExperience) {
				return fmt.Sprintf("Claims %s of %s; you gave %s.", yearsText(c.years), skill.Name, yearsText(*skill.YearsExperience)), true
			}
		}
		return "", false
	}
	if !basis.HasCareer {
		return fmt.Sprintf("Claims %s, and your work history has no dates to check it against.", yearsText(c.years)), true
	}
	if c.years > math.Round(basis.CareerYears) {
		return fmt.Sprintf("Claims %s; your work history covers about %s.", yearsText(c.years), yearsText(math.Round(basis.CareerYears))), true
	}
	return "", false
}

// saidThemselves reports whether the candidate's own words claim at least
// as many years about every skill the claim names.
func saidThemselves(c claim, own []claim) bool {
	for _, theirs := range own {
		if theirs.years < c.years {
			continue
		}
		covered := true
		for _, skill := range c.skills {
			if !slices.ContainsFunc(theirs.skills, func(s CandidateSkill) bool { return s.Name == skill.Name }) {
				covered = false
				break
			}
		}
		if covered {
			return true
		}
	}
	return false
}

func yearsText(years float64) string {
	n := strconv.FormatFloat(years, 'f', -1, 64)
	if years == 1 {
		return n + " year"
	}
	return n + " years"
}
