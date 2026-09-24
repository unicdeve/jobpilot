package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCredentialRef(t *testing.T) {
	t.Parallel()

	t.Run("an empty label becomes the default", func(t *testing.T) {
		t.Parallel()
		ref, err := NewCredentialRef("adzuna", "")
		require.NoError(t, err)
		assert.Equal(t, "default", ref.Label)
		assert.Equal(t, "adzuna/default", ref.String())
	})

	t.Run("surrounding whitespace is not part of a ref", func(t *testing.T) {
		t.Parallel()
		ref, err := NewCredentialRef("  adzuna  ", "  work  ")
		require.NoError(t, err)
		assert.Equal(t, "adzuna/work", ref.String())
	})

	for _, valid := range []string{"adzuna", "google-search", "a", "us-gov-jobs", "s3", "x1-y2"} {
		t.Run("valid: "+valid, func(t *testing.T) {
			t.Parallel()
			_, err := NewCredentialRef(valid, valid)
			assert.NoError(t, err)
		})
	}
}

// TestACredentialRefCannotBeAmbiguous is the reason the character set is
// restricted at all. A ref addresses one keychain item as "source/label",
// so a part holding a separator would make two different credentials the
// same item, and storing one would silently destroy the other.
func TestACredentialRefCannotBeAmbiguous(t *testing.T) {
	t.Parallel()

	_, withSlashInLabel := NewCredentialRef("adzuna", "work/eu")
	require.Error(t, withSlashInLabel, "a slash in a label would collide with another ref")

	first, err := NewCredentialRef("adzuna", "work")
	require.NoError(t, err)
	second, err := NewCredentialRef("adzuna-work", "eu")
	require.NoError(t, err)
	assert.NotEqual(t, first.String(), second.String(),
		"two different credentials must not address the same keychain item")
}

func TestACredentialRefRejectsWhatCannotRoundTrip(t *testing.T) {
	t.Parallel()

	for name, part := range map[string]string{
		"empty":               "",
		"a slash":             "a/b",
		"a space":             "a b",
		"upper case":          "Adzuna",
		"an underscore":       "a_b",
		"a leading hyphen":    "-a",
		"a trailing hyphen":   "a-",
		"consecutive hyphens": "a--b",
		"a dot":               "a.b",
		"a newline":           "a\nb",
		"a null byte":         "a\x00b",
		"longer than the max": strings.Repeat("a", maxCredentialPart+1),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, asSource := NewCredentialRef(part, "default")
			assert.Error(t, asSource, "should be rejected as a source")

			if part != "" {
				// An empty label is legal: it becomes "default".
				_, asLabel := NewCredentialRef("adzuna", part)
				assert.Error(t, asLabel, "should be rejected as a label")
			}
		})
	}
}

// TestEnvVarNameIsUnambiguous guards the second reason consecutive hyphens
// are forbidden. Each hyphen becomes an underscore, so two underscores can
// separate the parts only while no part can produce a pair on its own.
func TestEnvVarNameIsUnambiguous(t *testing.T) {
	t.Parallel()

	ref, err := NewCredentialRef("adzuna", "work-eu")
	require.NoError(t, err)
	assert.Equal(t, "JOBPILOT_CREDENTIAL_ADZUNA__WORK_EU", ref.EnvVarName())

	// The pair that would collide if a part could contain two underscores.
	a, err := NewCredentialRef("adzuna", "work-eu")
	require.NoError(t, err)
	b, err := NewCredentialRef("adzuna-work", "eu")
	require.NoError(t, err)
	assert.NotEqual(t, a.EnvVarName(), b.EnvVarName())

	// Every valid ref yields exactly one separator.
	for _, ref := range []CredentialRef{a, b} {
		assert.Equal(t, 1, strings.Count(strings.TrimPrefix(ref.EnvVarName(), "JOBPILOT_CREDENTIAL_"), "__"),
			"%s should split in exactly one place", ref.EnvVarName())
	}
}
