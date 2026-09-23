package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Date is a calendar date: a day, not an instant.
//
// A job started in July 2022. It did not start at midnight UTC on the 1st,
// and treating it as though it did is how a date becomes the day before in
// a negative-offset timezone. It also broke the API outright: an HTML date
// input sends "2022-07-01", and time.Time only accepts RFC 3339, so saving
// a work history from the desktop failed with a parse error.
//
// Dates therefore have one spelling everywhere — YYYY-MM-DD — in JSON, in
// SQLite, and on screen.
type Date struct {
	year  int
	month time.Month
	day   int
}

// dateLayout is the only spelling Date writes.
const dateLayout = "2006-01-02"

// dateInputLayouts are the spellings Date reads, most precise first.
//
// RFC 3339 is accepted because that is what rows written before dates had
// their own type contain, and because a client that sends a timestamp for
// a date means the day it falls on.
//
// A month without a day is accepted because a resume writes "2022-07" far
// more often than it writes a day, and the first of the month is the only
// reading of it.
var dateInputLayouts = []string{
	dateLayout,
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01",
	"2006",
}

// NewDate returns the given calendar date.
func NewDate(year int, month time.Month, day int) Date {
	return Date{year: year, month: month, day: day}
}

// DateOf returns the calendar date an instant falls on, in its own
// location.
func DateOf(t time.Time) Date {
	year, month, day := t.Date()
	return Date{year: year, month: month, day: day}
}

// ParseDate reads a date, reporting whether it could.
//
// A value it cannot read is not an error to shout about: a resume writes
// dates a dozen ways, and an entry whose date cannot be read is still
// worth keeping without one. It is a guessed date that would do harm.
func ParseDate(value string) (Date, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Date{}, false
	}
	for _, layout := range dateInputLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return DateOf(parsed), true
		}
	}
	return Date{}, false
}

// Year is the date's year.
func (d Date) Year() int { return d.year }

// Month is the date's month.
func (d Date) Month() time.Month { return d.month }

// Day is the date's day of the month.
func (d Date) Day() int { return d.day }

// Time returns midnight UTC on this date, for arithmetic and comparison.
func (d Date) Time() time.Time {
	return time.Date(d.year, d.month, d.day, 0, 0, 0, 0, time.UTC)
}

// IsZero reports whether the date is unset.
func (d Date) IsZero() bool { return d == Date{} }

// Before reports whether d falls before other.
func (d Date) Before(other Date) bool { return d.Time().Before(other.Time()) }

// After reports whether d falls after other.
func (d Date) After(other Date) bool { return d.Time().After(other.Time()) }

// Format renders the date with a time layout, for display.
func (d Date) Format(layout string) string { return d.Time().Format(layout) }

// String renders the date as YYYY-MM-DD.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.Format(dateLayout)
}

// MarshalJSON writes the date as "YYYY-MM-DD".
func (d Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON reads a date, accepting an empty string as no date.
//
// Empty is not an error because every form sends one for a field the user
// left blank, and rejecting the whole profile over a blank optional field
// is what this type exists to stop.
func (d *Date) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("a date must be a string like 2006-01-02: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		*d = Date{}
		return nil
	}
	parsed, ok := ParseDate(value)
	if !ok {
		return fmt.Errorf("%q is not a date; write it as 2006-01-02", value)
	}
	*d = parsed
	return nil
}

// Value stores the date as YYYY-MM-DD text, which SQLite compares and
// orders correctly as a string.
func (d Date) Value() (driver.Value, error) {
	if d.IsZero() {
		return nil, nil
	}
	return d.String(), nil
}

// Scan reads a date from SQLite.
//
// It accepts a time as well as text because rows written before dates had
// their own type hold a timestamp, and those rows must keep working.
func (d *Date) Scan(src any) error {
	switch value := src.(type) {
	case nil:
		*d = Date{}
		return nil
	case time.Time:
		*d = DateOf(value)
		return nil
	case string:
		return d.scanText(value)
	case []byte:
		return d.scanText(string(value))
	default:
		return fmt.Errorf("cannot read a date from %T", src)
	}
}

func (d *Date) scanText(value string) error {
	if strings.TrimSpace(value) == "" {
		*d = Date{}
		return nil
	}
	parsed, ok := ParseDate(value)
	if !ok {
		return fmt.Errorf("stored value %q is not a date", value)
	}
	*d = parsed
	return nil
}
