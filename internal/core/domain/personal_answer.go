package domain

import (
	"sort"
	"strings"
	"time"
	"unicode"
)

// PersonalAnswer is something the candidate said about themselves, in
// answer to a question: what they want next, a story, an answer they wrote
// on an application and kept (docs/02_features/16_about_you.md, ADR-0022).
//
// Only the candidate writes one (ADR-0034 §5).
type PersonalAnswer struct {
	ID          string            `json:"id"`
	CandidateID string            `json:"candidate_id"`
	Key         PersonalAnswerKey `json:"key"`
	Question    string            `json:"question"`
	Answer      string            `json:"answer"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// PersonalAnswerKey says what a PersonalAnswer is, so a question's wording
// can change without changing what its answer means.
type PersonalAnswerKey string

// The keys of a candidate's answers.
const (
	AnswerLookingFor   PersonalAnswerKey = "looking_for"
	AnswerValues       PersonalAnswerKey = "values"
	AnswerProudestWork PersonalAnswerKey = "proudest_work"
	AnswerKnownFor     PersonalAnswerKey = "known_for"
	AnswerCareerStory  PersonalAnswerKey = "career_story"
	// AnswerKept is an answer the candidate wrote to a question on an
	// application and chose to keep; its Question is that question.
	AnswerKept PersonalAnswerKey = "kept"
)

// StandingQuestion is one of the questions the candidate answers once, for
// every application.
type StandingQuestion struct {
	Key      PersonalAnswerKey `json:"key"`
	Question string            `json:"question"`
	// Hint says what a good answer covers, for whoever is asking.
	Hint string `json:"hint"`
}

// StandingQuestions are the About you questions, in the order they are
// asked. Their wording is here and nowhere else, so every client asks
// them the same way.
var StandingQuestions = []StandingQuestion{
	{
		Key:      AnswerLookingFor,
		Question: "What are you looking for in your next role?",
		Hint:     "The work you want to do, the problems you want to be near, what would make you glad you moved.",
	},
	{
		Key:      AnswerValues,
		Question: "What do you care about in your work, and in the people you do it with?",
		Hint:     "What you hold yourself to, and what makes a team one you want to be part of.",
	},
	{
		Key:      AnswerProudestWork,
		Question: "Tell us about the work you are proudest of.",
		Hint:     "What the problem was, what you did yourself, and what changed because of it.",
	},
	{
		Key:      AnswerKnownFor,
		Question: "What do colleagues come to you for?",
		Hint:     "The things people bring to you, in your own words.",
	},
	{
		Key:      AnswerCareerStory,
		Question: "How did you get into this work, and where are you taking it?",
		Hint:     "Include any change of direction or gap you would like explained your way.",
	},
}

// IsStandingKey reports whether key is one of the About you questions.
func IsStandingKey(key PersonalAnswerKey) bool {
	for _, question := range StandingQuestions {
		if question.Key == key {
			return true
		}
	}
	return false
}

// StandingQuestionFor returns the wording of a standing question.
func StandingQuestionFor(key PersonalAnswerKey) (StandingQuestion, bool) {
	for _, question := range StandingQuestions {
		if question.Key == key {
			return question, true
		}
	}
	return StandingQuestion{}, false
}

// maxKeptAnswersUsed is how many kept answers RelevantKeptAnswers returns.
const maxKeptAnswersUsed = 3

// RelevantKeptAnswers chooses the kept answers closest to a question:
// those whose question shares the most words with it, newest first on a
// tie, and none that share no word at all. It is deterministic, so why an
// answer was suggested can be told from the data alone.
func RelevantKeptAnswers(kept []PersonalAnswer, label string) []PersonalAnswer {
	want := significantWords(label)
	type scored struct {
		answer PersonalAnswer
		shared int
	}
	var candidates []scored
	for _, answer := range kept {
		shared := 0
		for word := range significantWords(answer.Question) {
			if want[word] {
				shared++
			}
		}
		if shared > 0 && strings.TrimSpace(answer.Answer) != "" {
			candidates = append(candidates, scored{answer, shared})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].shared != candidates[j].shared {
			return candidates[i].shared > candidates[j].shared
		}
		return candidates[i].answer.UpdatedAt.After(candidates[j].answer.UpdatedAt)
	})

	chosen := make([]PersonalAnswer, 0, maxKeptAnswersUsed)
	for i := 0; i < len(candidates) && i < maxKeptAnswersUsed; i++ {
		chosen = append(chosen, candidates[i].answer)
	}
	return chosen
}

// stopWords are too common in application questions to say two of them are
// about the same thing.
var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "about": true, "as": true, "at": true,
	"be": true, "by": true, "can": true, "describe": true, "do": true, "does": true,
	"for": true, "from": true, "have": true, "how": true, "i": true, "if": true, "in": true,
	"is": true, "it": true, "me": true, "of": true, "on": true, "or": true, "our": true,
	"please": true, "share": true, "tell": true, "that": true, "the": true, "this": true,
	"to": true, "us": true, "was": true, "we": true, "were": true, "what": true, "when": true,
	"where": true, "which": true, "who": true, "why": true, "with": true, "would": true,
	"you": true, "your": true,
}

func significantWords(text string) map[string]bool {
	words := map[string]bool{}
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(word) > 2 && !stopWords[word] {
			words[word] = true
		}
	}
	return words
}
