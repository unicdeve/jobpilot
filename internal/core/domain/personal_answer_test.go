package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRelevantKeptAnswers(t *testing.T) {
	t.Parallel()
	at := func(minutes int) time.Time { return time.Date(2026, 9, 29, 9, minutes, 0, 0, time.UTC) }
	kept := []PersonalAnswer{
		{ID: "why-old", Question: "Why do you want to work at Initech?", Answer: "Old", UpdatedAt: at(1)},
		{ID: "why-new", Question: "Why do you want to work at Globex?", Answer: "New", UpdatedAt: at(5)},
		{ID: "platform", Question: "Why do you want to join our platform team?", Answer: "Platform", UpdatedAt: at(2)},
		{ID: "review", Question: "How do you approach code review?", Answer: "Reviews", UpdatedAt: at(3)},
		{ID: "blank", Question: "Why do you want to work on our platform?", Answer: "  ", UpdatedAt: at(9)},
	}

	chosen := RelevantKeptAnswers(kept, "Why do you want to work on our platform team?")

	ids := make([]string, len(chosen))
	for i, answer := range chosen {
		ids[i] = answer.ID
	}
	assert.Equal(t, []string{"platform", "why-new", "why-old"}, ids,
		"most words shared first, newest on a tie, at most three, and never an empty answer")
	assert.Empty(t, RelevantKeptAnswers(kept, "What is your notice period?"), "nothing shared, nothing given")
	assert.Empty(t, RelevantKeptAnswers(kept, "Why do you?"), "words too common to mean anything are not shared words")
}
