// Package fold holds the one way a place name is reduced to a lookup key,
// shared by the geo tables and the command that generates them.
package fold

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Name reduces a place name to the form the tables are keyed by: lower
// case, accents removed, dots dropped, and anything else that is not a
// letter or a digit read as a space.
//
// Both sides of every comparison are folded the same way, so "Zürich",
// "Zurich" and "ZURICH" are one key, and so are "U.S." and "US". Dots are
// dropped rather than spaced because abbreviations are written with them;
// hyphens and apostrophes are spaced because names are joined with them,
// as in "Villeneuve-d'Ascq".
func Name(s string) string {
	stripped, _, err := transform.String(stripMarks(), s)
	if err != nil {
		stripped = s
	}
	var b strings.Builder
	b.Grow(len(stripped))
	space := false
	for _, r := range strings.ToLower(stripped) {
		switch {
		case r == '.':
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		default:
			space = true
		}
	}
	return b.String()
}

// stripMarks decomposes accented letters and drops the accents. A chain
// holds buffers, so each call gets its own: Name runs on every goroutine
// discovery asks a source from.
func stripMarks() transform.Transformer {
	return transform.Chain(norm.NFKD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
}
