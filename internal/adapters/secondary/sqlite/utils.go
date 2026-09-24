package sqlite

import (
	"database/sql"
	"encoding/json"
)

// nullableString converts an empty Go string (the domain-layer zero value
// for "not set") into a real SQL NULL, rather than storing an empty string
// literal in a column that other rows may leave NULL (e.g. a nullable
// foreign key like current_cover_letter_id).
func nullableString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// nullStringInto is a scan destination that reads a nullable TEXT column
// straight into a Go string, NULL becoming "".
func nullStringInto(dest *string) sql.Scanner {
	return nullStringScanner{dest}
}

type nullStringScanner struct{ dest *string }

func (s nullStringScanner) Scan(value any) error {
	var ns sql.NullString
	if err := ns.Scan(value); err != nil {
		return err
	}
	*s.dest = ns.String
	return nil
}

// joinStrings encodes s as a JSON array for storage in a TEXT column.
// Encoding a []string can never fail, so the error is safely ignored.
func joinStrings(s []string) string {
	if len(s) == 0 {
		return ""
	}
	b, _ := json.Marshal(s)
	return string(b)
}

// splitStrings decodes a TEXT column previously written by joinStrings back
// into a []string. Data that isn't valid JSON (e.g. rows written before this
// format was adopted) decodes to an empty slice rather than corrupting or
// panicking; this project has no released users yet, so there is no legacy
// data to migrate.
func splitStrings(s string) []string {
	if s == "" {
		return []string{}
	}
	var res []string
	if err := json.Unmarshal([]byte(s), &res); err != nil {
		return []string{}
	}
	return res
}
