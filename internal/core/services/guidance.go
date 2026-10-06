package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// guidanceSource is what a generator needs of the settings: the guidance
// its prompt is written under (ADR-0031).
type guidanceSource interface {
	PromptGuidance(ctx context.Context, key domain.PromptKey) (domain.PromptGuidance, error)
}

// guidanceFor returns the guidance a prompt is written under. A generator
// built without a source writes under the default.
func guidanceFor(ctx context.Context, source guidanceSource, key domain.PromptKey) (domain.PromptGuidance, error) {
	if source == nil {
		return domain.DefaultGuidance(key), nil
	}
	guidance, err := source.PromptGuidance(ctx, key)
	if err != nil {
		return domain.PromptGuidance{}, fmt.Errorf("failed to read the prompt's guidance: %w", err)
	}
	return guidance, nil
}

// rulesPrecedence opens the rules. The labels are plain words rather than
// markdown headings, as the posting the model reads is markdown. Measured
// on llama3.2:1b, two letters in four written under "### Guidance" and
// "### Rules" copied the posting's sections back; under plain labels none
// in eight did, and one in twenty under the prompt before guidance existed.
const rulesPrecedence = "These rules take precedence over the guidance above: "

// guidedSystemPrompt assembles a system prompt the user may give guidance
// to (ADR-0031 §3): the role, then the guidance, then the frame's rules,
// which come last and say they come first.
//
// The guidance is the user's own instruction, so it belongs here, in the
// one place the rules tell the model to take instructions from. It is
// never put in the user message, where it would sit beside a posting and
// look like the instructions a posting is not allowed to give.
func guidedSystemPrompt(role string, guidance domain.PromptGuidance, rules string) string {
	var sb strings.Builder
	sb.WriteString(role)
	sb.WriteString("\n\nGuidance:\n")
	sb.WriteString(guidance.Text())
	sb.WriteString("\n\n")
	sb.WriteString(rulesPrecedence)
	sb.WriteString(rules)
	return sb.String()
}
