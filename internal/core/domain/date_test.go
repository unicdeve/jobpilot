package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDate_JSON(t *testing.T) {
	t.Parallel()

	t.Run("reads what an HTML date input sends", func(t *testing.T) {
		t.Parallel()
		// This is the bug that made the whole profile unsaveable: the
		// desktop's date fields send YYYY-MM-DD, and time.Time accepts
		// only RFC 3339, so every work history was rejected with a parse
		// error the user could do nothing about.
		var experience CandidateExperience
		require.NoError(t, json.Unmarshal(
			[]byte(`{"company":"Acme","start_date":"2022-07-01"}`), &experience))

		require.NotNil(t, experience.StartDate)
		assert.Equal(t, 2022, experience.StartDate.Year())
		assert.Equal(t, time.July, experience.StartDate.Month())
		assert.Equal(t, 1, experience.StartDate.Day())
	})

	t.Run("an empty string is no date, not an error", func(t *testing.T) {
		t.Parallel()
		// Every form sends "" for a field left blank. Rejecting the whole
		// profile over one is what this type exists to stop.
		var experience CandidateExperience
		require.NoError(t, json.Unmarshal(
			[]byte(`{"company":"Acme","start_date":"","end_date":""}`), &experience))

		require.NotNil(t, experience.StartDate)
		assert.True(t, experience.StartDate.IsZero())
	})

	t.Run("null is no date", func(t *testing.T) {
		t.Parallel()
		var experience CandidateExperience
		require.NoError(t, json.Unmarshal(
			[]byte(`{"company":"Acme","start_date":null}`), &experience))
		assert.Nil(t, experience.StartDate)
	})

	t.Run("a timestamp is read as the day it falls on", func(t *testing.T) {
		t.Parallel()
		// Rows stored before dates had their own type hold a timestamp,
		// and a client may still send one.
		var experience CandidateExperience
		require.NoError(t, json.Unmarshal(
			[]byte(`{"start_date":"2022-07-01T00:00:00Z"}`), &experience))

		require.NotNil(t, experience.StartDate)
		assert.Equal(t, "2022-07-01", experience.StartDate.String())
	})

	t.Run("writes YYYY-MM-DD, which is what a date input reads back", func(t *testing.T) {
		t.Parallel()
		start := NewDate(2022, time.July, 1)
		encoded, err := json.Marshal(CandidateExperience{Company: "Acme", StartDate: &start})
		require.NoError(t, err)

		assert.Contains(t, string(encoded), `"start_date":"2022-07-01"`)
		assert.NotContains(t, string(encoded), `"start_date":"2022-07-01T`,
			"a date is a day; an instant is how it becomes the day before in another timezone")
	})

	t.Run("a value that is not a date says so usefully", func(t *testing.T) {
		t.Parallel()
		var experience CandidateExperience
		err := json.Unmarshal([]byte(`{"start_date":"last summer"}`), &experience)
		assert.ErrorContains(t, err, "last summer")
		assert.ErrorContains(t, err, "2006-01-02")
	})
}

func TestDate_SQL(t *testing.T) {
	t.Parallel()

	t.Run("stores as text SQLite can order", func(t *testing.T) {
		t.Parallel()
		value, err := NewDate(2022, time.July, 1).Value()
		require.NoError(t, err)
		assert.Equal(t, "2022-07-01", value)
	})

	t.Run("an unset date stores as NULL", func(t *testing.T) {
		t.Parallel()
		value, err := Date{}.Value()
		require.NoError(t, err)
		assert.Nil(t, value)
	})

	t.Run("reads back what earlier rows stored", func(t *testing.T) {
		t.Parallel()
		// Before this type, the driver wrote a time. Those rows must keep
		// working without a migration.
		for _, stored := range []any{
			"2022-07-01",
			[]byte("2022-07-01"),
			time.Date(2022, time.July, 1, 0, 0, 0, 0, time.UTC),
			"2022-07-01T00:00:00Z",
		} {
			var date Date
			require.NoError(t, date.Scan(stored), "%v", stored)
			assert.Equal(t, "2022-07-01", date.String())
		}
	})

	t.Run("NULL is an unset date", func(t *testing.T) {
		t.Parallel()
		var date Date
		require.NoError(t, date.Scan(nil))
		assert.True(t, date.IsZero())
	})
}

func TestParseDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"2022-07-01", "2022-07-01", true},
		{"2022-07", "2022-07-01", true},
		{"2022", "2022-01-01", true},
		{"2022-07-01T09:30:00Z", "2022-07-01", true},
		{"  2022-07-01  ", "2022-07-01", true},
		{"", "", false},
		{"July 2022", "", false},
		{"not a date", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, ok := ParseDate(tt.in)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, got.String())
			}
		})
	}
}

func TestParseDraftDate_ReadsHowResumesWrite(t *testing.T) {
	t.Parallel()

	// A resume is not an API. These spellings are read here rather than
	// widened into the wire format.
	for spelling, want := range map[string]string{
		"Jan 2022":     "2022-01-01",
		"January 2022": "2022-01-01",
		"03/2019":      "2019-03-01",
		"2019/03":      "2019-03-01",
		"2022-07":      "2022-07-01",
	} {
		got := ParseDraftDate(spelling)
		require.NotNil(t, got, "%q", spelling)
		assert.Equal(t, want, got.String(), "%q", spelling)
	}

	for _, unreadable := range []string{"", "Present", "current", "sometime in 2019"} {
		assert.Nil(t, ParseDraftDate(unreadable), "%q", unreadable)
	}
}
