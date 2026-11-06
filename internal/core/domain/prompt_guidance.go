package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// PromptKey names a prompt the user may give guidance to (ADR-0031 §1).
// Storage, the API and the CLI name a prompt by its key.
type PromptKey string

// The prompts a user may give guidance to. The others JobPilot sends are
// extraction, and are not offered. Cover letters and written answers had
// prompts here until ADR-0034, when JobPilot stopped writing them.
const (
	PromptMatch PromptKey = "match"
)

// MaxGuidanceLength is the most characters a guidance may have, so it
// cannot crowd a small local model's context out from under the material
// it reads (ADR-0031 §6).
const MaxGuidanceLength = 2000

// ErrInvalidGuidance is returned for guidance that cannot be kept.
var ErrInvalidGuidance = errors.New("invalid guidance")

// PromptDefinition is a prompt a user may give guidance to, as JobPilot
// ships it.
type PromptDefinition struct {
	Key     PromptKey
	Title   string
	UsedFor string
	// Default is the guidance the prompt is written under until the user
	// replaces it. It is judgement and style only; the rules that protect
	// the candidate and the reply's contract are the prompt's frame, which
	// is not here because nothing may change it (ADR-0031 §2).
	Default string
}

// promptDefinitions are the prompts, in the order they are shown.
// Matching had no style text before guidance could be changed, so its
// default states what "analyze the fit" already asked for.
var promptDefinitions = []PromptDefinition{
	{
		Key:     PromptMatch,
		Title:   "Matching",
		UsedFor: "Scoring how well a job fits you, and saying why. Scouts score with it too.",
		Default: "Judge the fit on the skills, experience and seniority the job asks for, " +
			"weighing what it requires above what it would merely like.",
	},
}

// PromptDefinitions returns the prompts a user may give guidance to, in the
// order they are shown.
func PromptDefinitions() []PromptDefinition {
	return append([]PromptDefinition(nil), promptDefinitions...)
}

// PromptDefinitionFor returns the prompt with the given key, and false for
// a key that is not one of them.
func PromptDefinitionFor(key PromptKey) (PromptDefinition, bool) {
	for _, definition := range promptDefinitions {
		if definition.Key == key {
			return definition, true
		}
	}
	return PromptDefinition{}, false
}

// GuidanceDigest identifies a default's text, so a change to it in a later
// release can be told from the one the user saw when they saved.
func GuidanceDigest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// StoredGuidance is what the user saved for one prompt.
type StoredGuidance struct {
	Text string `json:"text"`
	// DefaultDigest is the GuidanceDigest of the default the user was
	// shown when they saved (ADR-0031 §7).
	DefaultDigest string `json:"default_digest"`
}

// PromptGuidanceSettings is the user's guidance, by prompt. A prompt with
// no entry is written under its default.
type PromptGuidanceSettings map[PromptKey]StoredGuidance

// PromptGuidance is one prompt's guidance as the user sees it.
type PromptGuidance struct {
	Key     PromptKey `json:"key"`
	Title   string    `json:"title"`
	UsedFor string    `json:"used_for"`
	Default string    `json:"default"`
	// Custom is the user's guidance, empty while the default is in use.
	Custom string `json:"custom,omitempty"`
	// DefaultChanged is whether the default has changed since the user
	// saved Custom.
	DefaultChanged bool `json:"default_changed"`
}

// GuidanceOf is a prompt's guidance as the user's settings leave it.
func GuidanceOf(definition PromptDefinition, settings PromptGuidanceSettings) PromptGuidance {
	guidance := PromptGuidance{
		Key:     definition.Key,
		Title:   definition.Title,
		UsedFor: definition.UsedFor,
		Default: definition.Default,
	}
	if stored, ok := settings[definition.Key]; ok && stored.Text != "" {
		guidance.Custom = stored.Text
		guidance.DefaultChanged = stored.DefaultDigest != GuidanceDigest(definition.Default)
	}
	return guidance
}

// DefaultGuidance is a prompt's guidance before the user has changed it.
// It is what a generator built without the settings writes under.
func DefaultGuidance(key PromptKey) PromptGuidance {
	definition, _ := PromptDefinitionFor(key)
	return GuidanceOf(definition, nil)
}

// Text is the guidance the prompt is written under.
func (g PromptGuidance) Text() string {
	if g.Custom != "" {
		return g.Custom
	}
	return g.Default
}

// IsDefault reports whether the prompt is written under its default.
func (g PromptGuidance) IsDefault() bool {
	return g.Custom == ""
}

// NormalizeGuidance returns guidance as it is kept, or an error saying why
// it cannot be (ADR-0031 §6). The text is otherwise the user's, and is not
// read for what it asks.
func NormalizeGuidance(text string) (string, error) {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return "", fmt.Errorf("%w: it is empty; reset the prompt to go back to its default", ErrInvalidGuidance)
	}
	if n := utf8.RuneCountInString(text); n > MaxGuidanceLength {
		return "", fmt.Errorf("%w: it is %d characters, and can be at most %d", ErrInvalidGuidance, n, MaxGuidanceLength)
	}
	return text, nil
}
