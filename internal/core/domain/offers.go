package domain

import (
	"regexp"
	"strings"

	"github.com/unicdev/jobpilot/internal/core/domain/geo"
)

// OfferStatus is what a posting says about relocation or visa sponsorship
// (ADR-0023 §5). It is a label for the candidate, always shown with the
// sentence it came from, never a claim that a package exists.
type OfferStatus string

// The statuses an offer can have. Empty is a posting that does not mention it.
const (
	OfferOffered   OfferStatus = "offered"
	OfferMentioned OfferStatus = "mentioned"
	OfferDeclined  OfferStatus = "declined"
)

// Offer is what a posting says about one kind of help, and the sentence
// that said it.
type Offer struct {
	Status   OfferStatus `json:"status"`
	Evidence string      `json:"evidence,omitempty"`
}

// Available reports whether the posting offers it or mentions it without
// refusing it: what a search that includes relocation keeps.
func (o Offer) Available() bool {
	return o.Status == OfferOffered || o.Status == OfferMentioned
}

// offerTopic is what one kind of offer is about and how its offers read.
type offerTopic struct {
	// about reports whether a clause is about this kind of help at all.
	about func(clause string) bool
	// offered is phrasing that states an offer.
	offered *regexp.Regexp
}

// helpWords are what a refusal refuses.
const helpWords = `(offer|offering|provide|providing|support|supporting|sponsor|sponsoring|cover|covering|assist|consider|accommodate|help|include|relocat\w*|visas?|sponsorship)\b`

var (
	// clauseBreaks split a sentence where it can turn: "We do not sponsor
	// visas, but relocation support is available" refuses one thing and
	// offers another.
	clauseBreaks = regexp.MustCompile(`(?i)\b(but|however|although|though|whereas)\b|;`)

	// declinedCues are refusals, read on folded text, so "don't" is
	// "don t". A negation counts only where it governs the help itself —
	// "unable to offer … relocation support", "relocation will not be
	// provided" — so "we are not a typical company" refuses nothing.
	declinedCues = regexp.MustCompile(strings.Join([]string{
		`\b(unable to|cannot|can not|can t|cant|not able to|will not|won t|would not|not currently|no longer)( \w+){0,5} ` + helpWords,
		// "is not" and "do not" are everyday words, so they reach only as
		// far as "we do not currently offer".
		`\b(are not|is not|do not|does not|don t|doesn t)( \w+){0,2} ` + helpWords,
		`\b(relocation|sponsorship|visas?)( \w+){0,6} (is|are|will|can|would|be)( \w+)? not\b`,
		`\bnot (be )?(provided|offered|available|possible|eligible|included|covered|supported)\b`,
		`\bno (visa|visas|relocation|sponsorship|work permit)\b`,
		`\bwithout (\w+ ){0,4}(sponsorship|sponsor|relocat\w*|visas?)\b`,
	}, "|"))

	// conditionalCues make an offer uncertain: it may exist, for some.
	conditionalCues = regexp.MustCompile(`\b(may|might|could|possibly|potentially|case by case|position dependent|depending|dependent|where (possible|applicable)|if (needed|required|applicable|eligible)|for eligible|eligible candidates|subject to|transfer only|can be discussed|negotiable|in some cases)\b`)

	relocationTerm = regexp.MustCompile(`\brelocat(e|ion|ing)\b`)
	visaTerm       = regexp.MustCompile(`\bvisas?\b`)
	sponsorTerm    = regexp.MustCompile(`\bsponsor(s|ed|ing|ship)?\b`)
	// sponsorOtherwise is sponsorship that is not a visa's: "mentor,
	// sponsor and develop senior engineers", "company-sponsored dinners".
	sponsorOtherwise = regexp.MustCompile(`\b(company|employer|team|firm) sponsored\b|\bsponsored (dinners?|events?|memberships?|lunch(es)?|activities|retirement)\b|\bmentor\w* sponsor\b|\bsponsor and develop\b`)
	// sponsorWork is sponsorship in the words of work permission.
	sponsorWork = regexp.MustCompile(strings.Join([]string{
		`\b(work permit|right to work|work authori[sz]ation|authori[sz]ed to work|immigration|h 1 ?b|blue card|green card|relocat\w*|employment)\b`,
		`\b(offer|offers|offering|provide|provides|providing|support|supports|require|requires|requiring|need|needs|able to) (\w+ ){0,2}sponsor`,
		`\bsponsorship (is |are )?(available|offered|provided|possible|yes)\b`,
		`\b(no|without) (\w+ ){0,2}sponsorship\b`,
		`\bsponsor (visas|you|your)\b`,
	}, "|"))

	relocationOffered = regexp.MustCompile(strings.Join([]string{
		`\brelocation (package|support|assistance|allowance|bonus|stipend|budget|help|benefits?|costs?|expenses|reimbursement)\b`,
		`\brelocation (\w+ ){0,3}(covered|provided|offered|paid|available|included)\b`,
		`\b(offer|offers|offering|provide|provides|providing|cover|covers|support|supports|help|helps|assist|assists|pay for|pays for) (\w+ ){0,4}relocat`,
		`\bfull relocation\b`,
	}, "|"))

	visaOffered = regexp.MustCompile(strings.Join([]string{
		`\b(visa )?sponsorship (is )?(available|provided|offered|yes)\b`,
		`\bvisa (sponsorship|support|assistance|help)\b`,
		`\b(offer|offers|offering|provide|provides|providing|support|supports) (\w+ ){0,3}(visa|sponsorship)\b`,
		`\bwe (can|will|do) sponsor\b`,
		`\bsponsor (visas|your visa)\b`,
	}, "|"))

	relocationTopic = offerTopic{
		about:   func(clause string) bool { return relocationTerm.MatchString(clause) },
		offered: relocationOffered,
	}
	visaTopic = offerTopic{
		about: func(clause string) bool {
			if visaTerm.MatchString(clause) {
				return true
			}
			return sponsorTerm.MatchString(clause) && !sponsorOtherwise.MatchString(clause) &&
				sponsorWork.MatchString(clause)
		},
		offered: visaOffered,
	}
)

// offerRank orders statuses for a posting that says more than one thing.
// A refusal wins: a posting that refuses in one sentence and lists benefits
// "that may include relocation" in another has refused.
var offerRank = map[OfferStatus]int{OfferDeclined: 3, OfferOffered: 2, OfferMentioned: 1}

// readOffer reads a posting's sentences for one kind of help, clause by
// clause, and keeps the strongest status any clause has, with its sentence.
func readOffer(sentences []string, topic offerTopic) Offer {
	var best Offer
	for _, sentence := range sentences {
		question := strings.HasSuffix(strings.TrimSpace(sentence), "?")
		// Split before folding: folding turns the semicolon into a space.
		for _, raw := range clauseBreaks.Split(sentence, -1) {
			clause := geo.Fold(raw)
			if !topic.about(clause) {
				continue
			}
			status := classifyClause(clause, topic, question)
			if offerRank[status] > offerRank[best.Status] {
				best = Offer{Status: status, Evidence: evidence(sentence)}
			}
		}
	}
	return best
}

// classifyClause is one clause's status: a refusal anywhere in it declines;
// a conditional, a question or a requirement at the candidate's cost only
// mentions; an offer stated plainly offers.
func classifyClause(clause string, topic offerTopic, question bool) OfferStatus {
	switch {
	case declinedCues.MatchString(clause):
		return OfferDeclined
	case question, conditionalCues.MatchString(clause):
		return OfferMentioned
	case topic.offered.MatchString(clause):
		return OfferOffered
	}
	return OfferMentioned
}
