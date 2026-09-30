package domain

import (
	"html"
	"regexp"
	"strings"
)

// SignalsVersion is the version of the reading ReadSignals does. A job
// stored with an older version is read again when the daemon starts
// (ADR-0023 §3), so raising it whenever the vocabulary, the tables or the
// phrases change reaches every stored job, not only new ones.
const SignalsVersion = 2

// ReadSignals derives what a job says about itself beyond its fields: where
// it can be done from, and whether it offers relocation and visa
// sponsorship (ADR-0023). It reads only what the job already holds, and no
// model: the discovery path reads several hundred jobs per search, from
// text nobody at JobPilot wrote.
func ReadSignals(job *Job) {
	sentences := Sentences(job.DescriptionMarkdown)
	job.Workable = readWorkable(*job, sentences)
	job.Relocation = readOffer(sentences, relocationTopic)
	job.VisaSponsorship = readOffer(sentences, visaTopic)
	job.SignalsVersion = SignalsVersion
}

var (
	htmlTag = regexp.MustCompile(`(?s)<[^>]*>`)
	// markdownLink is "[text](url)", which a sentence keeps as its text.
	markdownLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	// markdownMarks are the characters markdown uses for emphasis,
	// headings and escapes, which a sentence quoted back to the user does
	// not need.
	markdownMarks = strings.NewReplacer("**", "", "__", "", "\\", "", "#", "", "`", "")
	// abbreviations are dotted forms that would otherwise end a sentence.
	abbreviations = strings.NewReplacer(
		"U.S.A.", "USA", "U.S.", "US", "U.K.", "UK", "U.A.E.", "UAE",
		"e.g.", "eg", "i.e.", "ie", "etc.", "etc", "approx.", "approx", "incl.", "incl",
	)
	// sentenceEnd also splits at " | ", which is how a Hacker News posting
	// separates its fields: "ONSITE | Full-time | Visa sponsorship available".
	sentenceEnd = regexp.MustCompile(`[.!?]+(?:\s+|$)|\n+|\s[-*•]\s|\s\|\s`)
	whitespace  = regexp.MustCompile(`\s+`)
)

// maxEvidence bounds a sentence kept as evidence. A posting written as one
// long paragraph has no sentence boundary to find, and the user needs the
// part that says it, not the posting.
const maxEvidence = 280

// Sentences splits a posting into its sentences as plain text: HTML and
// markdown removed, a list item or a line its own sentence.
func Sentences(description string) []string {
	text := html.UnescapeString(description)
	// Descriptions from some sources arrive escaped twice.
	text = html.UnescapeString(text)
	text = htmlTag.ReplaceAllString(text, "\n")
	text = markdownLink.ReplaceAllString(text, "$1")
	text = markdownMarks.Replace(text)
	text = abbreviations.Replace(text)

	parts := sentenceEnd.Split(text, -1)
	sentences := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(whitespace.ReplaceAllString(part, " "))
		part = strings.TrimLeft(part, "-*•> ")
		if part != "" {
			sentences = append(sentences, part)
		}
	}
	return sentences
}

// evidence is a sentence as it is kept and shown: whole, unless it is too
// long to read at a glance.
func evidence(sentence string) string {
	if len(sentence) <= maxEvidence {
		return sentence
	}
	cut := strings.LastIndex(sentence[:maxEvidence], " ")
	if cut <= 0 {
		cut = maxEvidence
	}
	return sentence[:cut] + "…"
}
