package sqlite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDSN_CarriesEveryConnectionPragma(t *testing.T) {
	t.Parallel()

	got, err := dsn("/tmp/jobpilot/test.db")
	require.NoError(t, err)

	// The path must survive verbatim: the driver only percent-decodes a DSN
	// that opens with "file:", and this one deliberately does not.
	assert.True(t, len(got) > len("/tmp/jobpilot/test.db"))
	assert.Equal(t, "/tmp/jobpilot/test.db", got[:len("/tmp/jobpilot/test.db")])

	for _, want := range []string{"_busy_timeout=5000", "_journal_mode=WAL", "_foreign_keys=on", "_time_format=sqlite", "_timezone=UTC", "_pragma=secure_delete%28on%29"} {
		assert.Contains(t, got, want)
	}
}

func TestDSN_PreservesWindowsStylePath(t *testing.T) {
	t.Parallel()

	got, err := dsn(`C:\Users\jobpilot\data\jobpilot.db`)
	require.NoError(t, err)
	assert.Contains(t, got, `C:\Users\jobpilot\data\jobpilot.db?`)
}

func TestDSN_RejectsPathContainingQuestionMark(t *testing.T) {
	t.Parallel()

	// The driver splits the DSN at the first "?", so such a path would
	// silently open a different database rather than the one requested.
	_, err := dsn("/tmp/what?/test.db")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contains '?'")
}
