package services

import "strings"

// nonEmpty drops blank entries from a list the model produced.
//
// A model asked for a list of strings will happily return [""] for "none",
// and an empty bullet shown to the user as a strength or a gap is noise
// that reads like a bug. This is the smallest example of why every reply
// is checked after decoding: a schema guarantees a list of strings, not a
// list of meaningful ones.
func nonEmpty(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}
