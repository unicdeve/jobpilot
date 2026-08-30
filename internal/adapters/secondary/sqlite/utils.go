package sqlite

import "strings"

func joinStrings(s []string) string {
	if len(s) == 0 {
		return ""
	}
	var res strings.Builder
	for i, v := range s {
		if i > 0 {
			res.WriteString(",")
		}
		res.WriteString(v)
	}
	return res.String()
}

func splitStrings(s string) []string {
	if s == "" {
		return []string{}
	}
	var res []string
	var current string
	inQuotes := false

	for _, r := range s {
		if r == '"' {
			inQuotes = !inQuotes
		} else if r == ',' && !inQuotes {
			res = append(res, current)
			current = ""
		} else {
			current += string(r)
		}
	}
	res = append(res, current)
	return res
}
