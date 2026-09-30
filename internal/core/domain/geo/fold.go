package geo

import "github.com/unicdev/jobpilot/internal/core/domain/geo/internal/fold"

// Fold reduces a place name to the form the tables are keyed by: lower
// case, accents removed, dots dropped, and anything else that is not a
// letter or a digit read as a space. "Zürich", "Zurich" and "ZURICH" fold
// alike, and so do "U.S." and "US".
func Fold(s string) string { return fold.Name(s) }
