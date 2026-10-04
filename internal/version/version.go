// Package version is the version of JobPilot a binary was built as. The
// daemon and the CLI both import it, and a release sets it at build time
// (ADR-0029 §2):
//
//	go build -ldflags "-X github.com/unicdev/jobpilot/internal/version.Version=1.2.3"
//
// A binary built any other way, including by `make build` and `make dev`,
// says Dev.
package version

import (
	"slices"
	"strconv"
	"strings"
)

// Dev is the version of a binary no release built.
const Dev = "dev"

// Version is this binary's version, without the tag's "v".
var Version = Dev

// Compare orders two versions as semantic versions: -1 when a is older
// than b, 0 when they are the same, and 1 when a is newer.
//
// Either one being Dev, or not a semantic version, compares as the same,
// so a development build works against any other build, and nothing is
// stopped on a version it cannot read (ADR-0029 §4).
func Compare(a, b string) int {
	va, okA := parse(a)
	vb, okB := parse(b)
	if !okA || !okB {
		return 0
	}
	for i := range va.core {
		if va.core[i] != vb.core[i] {
			return compareInts(va.core[i], vb.core[i])
		}
	}
	return comparePrerelease(va.prerelease, vb.prerelease)
}

type semver struct {
	core       [3]uint64
	prerelease []string
}

// parse reads MAJOR.MINOR.PATCH, with an optional "-prerelease" and an
// optional "+build", which semantic versioning says is ignored when
// ordering. A leading "v" is accepted, as a tag has one.
func parse(s string) (semver, bool) {
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var v semver
	if i := strings.IndexByte(s, '-'); i >= 0 {
		v.prerelease = strings.Split(s[i+1:], ".")
		if slices.Contains(v.prerelease, "") {
			return semver{}, false
		}
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	for i, part := range parts {
		if !isNumeric(part) || (len(part) > 1 && part[0] == '0') {
			return semver{}, false
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return semver{}, false
		}
		v.core[i] = n
	}
	return v, true
}

// comparePrerelease orders pre-release identifiers as semantic versioning
// does: a version without one is newer than any with one, numeric
// identifiers compare as numbers and below any alphanumeric one, and a
// longer list is newer when the shorter is its prefix.
func comparePrerelease(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1
	case len(b) == 0:
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] == b[i] {
			continue
		}
		numA, numB := isNumeric(a[i]), isNumeric(b[i])
		switch {
		case numA && numB:
			na, _ := strconv.ParseUint(a[i], 10, 64)
			nb, _ := strconv.ParseUint(b[i], 10, 64)
			return compareInts(na, nb)
		case numA:
			return -1
		case numB:
			return 1
		default:
			return strings.Compare(a[i], b[i])
		}
	}
	return compareInts(uint64(len(a)), uint64(len(b)))
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func compareInts(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
